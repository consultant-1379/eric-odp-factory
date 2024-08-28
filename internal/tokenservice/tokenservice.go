package tokenservice

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"eric-odp-factory/internal/certwatcher"
	"eric-odp-factory/internal/common"
	"eric-odp-factory/internal/config"
)

const (
	defaultRequestTimeout        = 30
	defaultDialTimeout           = 5
	defaultIdleConnectionTimeout = 120
	defaultMaxIdleConnections    = 5

	applicationJSON = "application/json"
	requestFailed   = "request failed: %w"
)

var (
	errNoContentReturned     = errors.New("no content returned")
	errUnexpectedStatusCode  = errors.New("unexpected status code")
	errUnexpectedContentType = errors.New("unexpected content-type")
)

type OdpToken struct {
	Name string
	Data map[string]string
}

type Client interface {
	CreateToken(ctx context.Context, username string, tokentypes []string) (*OdpToken, error)
	GetToken(ctx context.Context, name string) (*OdpToken, error)
	DeleteToken(ctx context.Context, name string) error
}

type ClientImpl struct {
	url        string
	httpClient *http.Client
}

type ODPTokenGenerateParams struct {
	UserName   string   `json:"username"`
	TokenTypes []string `json:"tokentypes"`
}

type ODPTokenAuthData struct {
	TokenName string            `json:"tokenname"`
	TokenData map[string]string `json:"tokendata"`
}

func NewTokenServiceClient(cfg *config.Config) *ClientImpl {
	slog.Info("NewTokenServiceClient", "TokenServiceURL", cfg.TokenServiceURL)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.IdleConnTimeout = defaultIdleConnectionTimeout * time.Second
	transport.MaxIdleConns = defaultMaxIdleConnections
	transport.MaxIdleConnsPerHost = defaultMaxIdleConnections

	if cfg.TokenServiceCAFile != "" {
		transport.DialContext = (&net.Dialer{
			Timeout: defaultDialTimeout * time.Second,
		}).DialContext

		fullpath, _ := filepath.Abs(cfg.TokenServiceCertFile)
		certDir := filepath.Dir(fullpath)
		certFileName := filepath.Base(cfg.TokenServiceCertFile)
		certKeyName := filepath.Base(cfg.TokenServiceKeyFile)
		certWatcher := certwatcher.NewWatcher(certDir, certFileName, certKeyName)

		caCert, err := os.ReadFile(cfg.TokenServiceCAFile)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := tls.Config{
			MinVersion:           tls.VersionTLS12,
			GetClientCertificate: certWatcher.GetClientCertificate,
			RootCAs:              caCertPool,
		}
		transport.TLSClientConfig = &tlsConfig
	} else {
		transport.DialContext = (&net.Dialer{
			Timeout: defaultDialTimeout * time.Second,
		}).DialContext
	}

	client := http.Client{
		Timeout:   defaultRequestTimeout * time.Second,
		Transport: setupMetrics(transport),
	}

	tsc := ClientImpl{
		url:        cfg.TokenServiceURL,
		httpClient: &client,
	}

	return &tsc
}

func (tsc *ClientImpl) CreateToken(ctx context.Context, username string, tokentypes []string) (*OdpToken, error) {
	slog.Info("CreateToken", common.CtxIDLabel, ctx.Value(common.CtxID),
		"username", username, "tokentypes", tokentypes)

	requestData := ODPTokenGenerateParams{UserName: username, TokenTypes: tokentypes}
	jsonData, _ := json.Marshal(requestData)

	odpToken, err := tsc.doRequest(ctx, http.MethodPost, "", jsonData)
	if err != nil {
		slog.Error("CreateToken request failed", common.CtxIDLabel, ctx.Value(common.CtxID),
			"username", username, "tokentypes", tokentypes, "err", err)

		return nil, fmt.Errorf(requestFailed, err)
	}

	return odpToken, nil
}

func (tsc *ClientImpl) GetToken(ctx context.Context, tokenname string) (*OdpToken, error) {
	slog.Info("GetToken", common.CtxIDLabel, ctx.Value(common.CtxID),
		"tokenname", tokenname)

	odpToken, err := tsc.doRequest(ctx, http.MethodGet, tokenname, nil)
	if err != nil {
		slog.Error("GetToken request failed", common.CtxIDLabel, ctx.Value(common.CtxID),
			"tokenname", tokenname, "err", err)

		return nil, fmt.Errorf(requestFailed, err)
	}

	return odpToken, nil
}

func (tsc *ClientImpl) DeleteToken(ctx context.Context, tokenname string) error {
	slog.Info("DeleteToken", common.CtxIDLabel, ctx.Value(common.CtxID),
		"tokenname", tokenname)

	_, err := tsc.doRequest(ctx, http.MethodDelete, tokenname, nil)
	if err != nil {
		slog.Error("DeleteToken request failed", common.CtxIDLabel, ctx.Value(common.CtxID),
			"tokenname", tokenname, "err", err)

		return fmt.Errorf(requestFailed, err)
	}

	return nil
}

func (tsc *ClientImpl) doRequest(ctx context.Context, method string, tokenname string, data []byte) (*OdpToken, error) {
	var requestBody io.Reader
	if data != nil {
		requestBody = bytes.NewReader(data)
	} else {
		requestBody = http.NoBody
	}
	request, err := http.NewRequestWithContext(ctx, method, tsc.url, requestBody)
	if err != nil {
		slog.Error("doRequest failed to create request", common.CtxIDLabel, ctx.Value(common.CtxID),
			"method", method, "err", err)

		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if data != nil {
		request.Header.Add("Content-Type", applicationJSON)
	}
	request.Header.Add("Accept", applicationJSON)

	if tokenname != "" {
		request.URL.Path = path.Join(request.URL.Path, tokenname)
	}

	slog.Debug("doRequest", common.CtxIDLabel, ctx.Value(common.CtxID),
		"request.URL", request.URL, "data", data)

	response, err := tsc.httpClient.Do(request)
	if err != nil {
		slog.Error("doRequest Do returned error", common.CtxIDLabel, ctx.Value(common.CtxID),
			"request", request, "err", err)
		dumpRequestAndResponse(ctx, request, data, nil, nil)

		return nil, fmt.Errorf("do returned error: %w", err)
	}

	return processResponse(ctx, request, data, response)
}

func processResponse(
	ctx context.Context,
	request *http.Request,
	requestData []byte,
	response *http.Response,
) (*OdpToken, error) {
	defer response.Body.Close()
	responseContent, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Error("processResponse error while reading response body", common.CtxIDLabel, ctx.Value(common.CtxID),
			"method", request.Method, "err", err)
		dumpRequestAndResponse(ctx, request, requestData, response, nil)

		return nil, fmt.Errorf("error while reading response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		slog.Error("processResponse unexpected status code", common.CtxIDLabel, ctx.Value(common.CtxID),
			"method", request.Method, "status", response.StatusCode)
		dumpRequestAndResponse(ctx, request, requestData, response, responseContent)

		return nil, fmt.Errorf("%w: %d", errUnexpectedStatusCode, response.StatusCode)
	}

	slog.Debug("processResponse", common.CtxIDLabel, ctx.Value(common.CtxID), "response.Header", response.Header)

	var odpToken *OdpToken
	if len(responseContent) > 0 {
		slog.Debug("processResponse", common.CtxIDLabel, ctx.Value(common.CtxID), "responseContent", responseContent)
		contentType := response.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, applicationJSON) {
			authData := ODPTokenAuthData{}
			err = json.NewDecoder(bytes.NewReader(responseContent)).Decode(&authData)
			if err != nil {
				slog.Error("processResponse decode response body failed", common.CtxIDLabel, ctx.Value(common.CtxID),
					"method", request.Method, "err", err)
				dumpRequestAndResponse(ctx, request, requestData, response, responseContent)

				return nil, fmt.Errorf("decode response body failed: %w", err)
			}
			odpToken = &OdpToken{Name: authData.TokenName, Data: authData.TokenData}
		} else {
			slog.Error("processResponse unexpected Content-Type", common.CtxIDLabel, ctx.Value(common.CtxID),
				"method", request.Method, "content-type", contentType)
			dumpRequestAndResponse(ctx, request, requestData, response, responseContent)

			return nil, fmt.Errorf("%w: %s", errUnexpectedContentType, contentType)
		}
	} else if request.Method == http.MethodPost || request.Method == http.MethodGet {
		slog.Error("processResponse no content returned", common.CtxIDLabel, ctx.Value(common.CtxID),
			"method", request.Method)
		dumpRequestAndResponse(ctx, request, requestData, response, responseContent)

		return nil, errNoContentReturned
	}

	return odpToken, nil
}

func dumpRequestAndResponse(ctx context.Context,
	request *http.Request, requestData []byte,
	response *http.Response, responseData []byte,
) {
	if reqDump, dumpErr := httputil.DumpRequest(request, false); dumpErr == nil {
		slog.Error("dumpRequestAndResponse", common.CtxIDLabel, ctx.Value(common.CtxID),
			"request", string(reqDump))
	} else {
		slog.Error("dumpRequestAndResponse failed to dump request", "dumpErr", dumpErr)
	}
	if requestData != nil {
		slog.Error("dumpRequestAndResponse", common.CtxIDLabel, ctx.Value(common.CtxID),
			"requestData", string(requestData))
	}

	if response != nil {
		if respDump, dumpErr := httputil.DumpResponse(response, false); dumpErr == nil {
			slog.Error("dumpRequestAndResponse", common.CtxIDLabel, ctx.Value(common.CtxID),
				"response", string(respDump))
		} else {
			slog.Error("dumpRequestAndResponse failed to dump response", "dumpErr", dumpErr)
		}
	}

	if responseData != nil {
		slog.Error("dumpRequestAndResponse", common.CtxIDLabel, ctx.Value(common.CtxID),
			"responseData", string(responseData))
	}
}
