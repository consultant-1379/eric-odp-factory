package userlookup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"reflect"
	"testing"
	"time"

	ldap "github.com/go-ldap/ldap/v3"

	"eric-odp-factory/internal/config"
)

const (
	CaFile = "/ca.crt"
)

type LdapTestSearchResult struct {
	sr  *ldap.SearchResult
	err error
}
type LdapTestClient struct {
	searchResults []LdapTestSearchResult
}

var errSearchFailed = errors.New("Search for user failed")

var testCtx = context.WithValue(context.TODO(), "ctxid", "test") //nolint:revive,staticcheck // Ignore for test code

func (ltc *LdapTestClient) Start() {}

func (ltc *LdapTestClient) StartTLS(*tls.Config) error {
	return nil
}

func (ltc *LdapTestClient) Close() error {
	return nil
}

func (ltc *LdapTestClient) GetLastError() error {
	return nil
}

func (ltc *LdapTestClient) IsClosing() bool {
	return false
}

func (ltc *LdapTestClient) SetTimeout(time.Duration) {}

func (ltc *LdapTestClient) TLSConnectionState() (tls.ConnectionState, bool) {
	return tls.ConnectionState{}, false
}

func (ltc *LdapTestClient) Bind(_, _ string) error {
	return nil
}

func (ltc *LdapTestClient) UnauthenticatedBind(_ string) error {
	return nil
}

func (ltc *LdapTestClient) SimpleBind(*ldap.SimpleBindRequest) (*ldap.SimpleBindResult, error) {
	return nil, nil
}

func (ltc *LdapTestClient) ExternalBind() error {
	return nil
}

func (ltc *LdapTestClient) NTLMUnauthenticatedBind(_, _ string) error {
	return nil
}

func (ltc *LdapTestClient) Unbind() error {
	return nil
}

func (ltc *LdapTestClient) Add(*ldap.AddRequest) error {
	return nil
}

func (ltc *LdapTestClient) Del(*ldap.DelRequest) error {
	return nil
}

func (ltc *LdapTestClient) Modify(*ldap.ModifyRequest) error {
	return nil
}

func (ltc *LdapTestClient) ModifyDN(*ldap.ModifyDNRequest) error {
	return nil
}

func (ltc *LdapTestClient) ModifyWithResult(*ldap.ModifyRequest) (*ldap.ModifyResult, error) {
	return nil, nil
}

func (ltc *LdapTestClient) Compare(_, _, _ string) (bool, error) {
	return false, nil
}

func (ltc *LdapTestClient) PasswordModify(_ *ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error) {
	return nil, nil
}

func (ltc *LdapTestClient) Search(_ *ldap.SearchRequest) (*ldap.SearchResult, error) {
	result, remainder := ltc.searchResults[0], ltc.searchResults[1:]
	ltc.searchResults = remainder

	return result.sr, result.err
}

func (ltc *LdapTestClient) SearchAsync(_ context.Context, _ *ldap.SearchRequest, _ int) ldap.Response {
	return nil
}

func (ltc *LdapTestClient) SearchWithPaging(_ *ldap.SearchRequest, _ uint32) (*ldap.SearchResult, error) {
	return nil, nil
}

func (ltc *LdapTestClient) DirSync(_ *ldap.SearchRequest, _, _ int64, _ []byte) (*ldap.SearchResult, error) {
	return nil, nil
}

//nolint:lll // Suppress long line
func (ltc *LdapTestClient) DirSyncAsync(_ context.Context, _ *ldap.SearchRequest, _ int, _, _ int64, _ []byte) ldap.Response {
	return nil
}

//nolint:lll // Suppress long line
func (ltc *LdapTestClient) Syncrepl(_ context.Context, _ *ldap.SearchRequest, _ int, _ ldap.ControlSyncRequestMode, _ []byte, _ bool) ldap.Response {
	return nil
}

var OkaySearchResults = []LdapTestSearchResult{
	{
		err: nil,
		sr: &ldap.SearchResult{
			Entries: []*ldap.Entry{
				{
					DN: "user=testuser,ou=users,dc=example,dc=org",
					Attributes: []*ldap.EntryAttribute{
						{
							Name:   "uidNumber",
							Values: []string{"1"},
						},
						{
							Name:   "gidNumber",
							Values: []string{"1"},
						},
						{
							Name:   "description",
							Values: []string{"desc1", "desc2"},
						},
					},
				},
			},
		},
	},
	{
		err: nil,
		sr: &ldap.SearchResult{
			Entries: []*ldap.Entry{
				{
					DN: "group=grp1,ou=groups,dc=example,dc=org",
					Attributes: []*ldap.EntryAttribute{
						{
							Name:   "gidNumber",
							Values: []string{"1"},
						},
						{
							Name:   "cn",
							Values: []string{"grp1"},
						},
					},
				},
				{
					DN: "group=grp2,ou=groups,dc=example,dc=org",
					Attributes: []*ldap.EntryAttribute{
						{
							Name:   "gidNumber",
							Values: []string{"2"},
						},
						{
							Name:   "cn",
							Values: []string{"grp2"},
						},
					},
				},
			},
		},
	},
}

func createUserLookup(t *testing.T) *Production {
	credsDir := t.TempDir()
	os.WriteFile(credsDir+"/"+usernameFile, []byte("ldapuser"), 0o644)
	os.WriteFile(credsDir+"/"+passwordFile, []byte("ldappassword"), 0o644)

	cfg := config.Config{
		LdapURL:         "NotUsed",
		LdapUserQuery:   "(uid=%s)",
		LdapUserBaseDN:  "ou=users,dc=example,dc=org",
		LdapUserAttrs:   "uidNumber,gidNumber,description",
		LdapGroupBaseDN: "ou=groups,dc=example,dc=org",
		LdapGroupQuery:  "(memberUid=%s)",
		LdapGroupAttrs:  "cn,gidNumber",
		LdapCredsDir:    credsDir,
	}
	ul, newError := NewUserLookup(&cfg)
	if newError != nil {
		t.Fatalf("TestLdapOkay: got unexpected error calling NewUserLookup %v", newError)
	}

	return ul
}

func TestLdapOkay(t *testing.T) {
	ul := createUserLookup(t)

	ul.testLdap = &LdapTestClient{searchResults: OkaySearchResults}

	userInfo, lookupErr := ul.Lookup(testCtx, "testuser")
	if lookupErr != nil {
		t.Errorf("TestLdapOkay: got unexpected error calling Lookup %v", lookupErr)

		return
	}

	if userInfo.Username != "testuser" {
		t.Errorf("TestLdapOkay: expected testuser got %s", userInfo.Username)
	}

	expectedUserAttr := map[string]string{
		"uidNumber":   "1",
		"gidNumber":   "2",
		"description": "desc1,desc2",
	}
	if reflect.DeepEqual(userInfo.UserAttr, expectedUserAttr) {
		t.Errorf("TestLdapOkay: expected %v got %v", expectedUserAttr, userInfo.UserAttr)
	}

	expectedGroupAttr := "grp1:1;grp2:2"
	if userInfo.Groups != expectedGroupAttr {
		t.Errorf("TestLdapOkay: expected %v got %v", expectedGroupAttr, userInfo.Groups)
	}
}

func TestNewUserLookupTLS(t *testing.T) {
	credsDir := t.TempDir()
	os.WriteFile(credsDir+"/"+usernameFile, []byte("ldapuser"), 0o644)
	os.WriteFile(credsDir+"/"+passwordFile, []byte("ldappassword"), 0o644)

	// Create CA
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Country:            []string{"SE"},
			Organization:       []string{"Ericsson"},
			OrganizationalUnit: []string{"OSS"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		panic(err)
	}

	// pem encode
	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	err = os.WriteFile(credsDir+CaFile, caPEM.Bytes(), 0o600)
	if err != nil {
		panic(err)
	}

	cfg := config.Config{
		LdapURL:         "NotUsed",
		LdapUserQuery:   "(uid=%s)",
		LdapUserBaseDN:  "ou=users,dc=example,dc=org",
		LdapUserAttrs:   "uidNumber,gidNumber,description",
		LdapGroupBaseDN: "ou=groups,dc=example,dc=org",
		LdapGroupQuery:  "(memberUid=%s)",
		LdapGroupAttrs:  "cn,gidNumber",
		LdapCredsDir:    credsDir,
		LdapCAFile:      credsDir + CaFile,
	}
	_, newError := NewUserLookup(&cfg)
	if newError != nil {
		t.Errorf("TestNewUserLookupTLS: unexpected error %v", newError)
	}
}

func TestNewUserLookupMissingUsername(t *testing.T) {
	credsDir := t.TempDir()

	cfg := config.Config{
		LdapURL:         "NotUsed",
		LdapUserQuery:   "(uid=%s)",
		LdapUserBaseDN:  "ou=users,dc=example,dc=org",
		LdapUserAttrs:   "uidNumber,gidNumber,description",
		LdapGroupBaseDN: "ou=groups,dc=example,dc=org",
		LdapGroupQuery:  "(memberUid=%s)",
		LdapGroupAttrs:  "cn,gidNumber",
		LdapCredsDir:    credsDir,
	}
	_, newError := NewUserLookup(&cfg)
	if newError == nil {
		t.Error("TestNewUserLookupTlsMissingCA: exported error, got none")
	}
}

func TestNewUserLookupTlsMissingCA(t *testing.T) {
	credsDir := t.TempDir()
	os.WriteFile(credsDir+"/"+usernameFile, []byte("ldapuser"), 0o644)
	os.WriteFile(credsDir+"/"+passwordFile, []byte("ldappassword"), 0o644)

	cfg := config.Config{
		LdapURL:         "NotUsed",
		LdapUserQuery:   "(uid=%s)",
		LdapUserBaseDN:  "ou=users,dc=example,dc=org",
		LdapUserAttrs:   "uidNumber,gidNumber,description",
		LdapGroupBaseDN: "ou=groups,dc=example,dc=org",
		LdapGroupQuery:  "(memberUid=%s)",
		LdapGroupAttrs:  "cn,gidNumber",
		LdapCredsDir:    credsDir,
		LdapCAFile:      credsDir + CaFile,
	}
	_, newError := NewUserLookup(&cfg)
	if newError == nil {
		t.Error("TestNewUserLookupTlsMissingCA: exported error, got none")
	}
}

func TestLdapSearchUserFail(t *testing.T) {
	ul := createUserLookup(t)

	ul.testLdap = &LdapTestClient{
		searchResults: []LdapTestSearchResult{
			{
				err: errSearchFailed,
			},
		},
	}

	expectedError := "failed search for user: Search for user failed"
	_, lookupErr := ul.Lookup(testCtx, "testuser")
	if lookupErr == nil || lookupErr.Error() != expectedError {
		t.Errorf("TestLdapSearchUserFail: got unexpected error calling Lookup %v", lookupErr)
	}
}

func TestLdapSearchUserNotFound(t *testing.T) {
	ul := createUserLookup(t)

	ul.testLdap = &LdapTestClient{
		searchResults: []LdapTestSearchResult{
			{
				err: nil,
				sr: &ldap.SearchResult{
					Entries: []*ldap.Entry{},
				},
			},
		},
	}

	expectedError := "invalid number of search results 0: invalid LDAP search result"
	_, lookupErr := ul.Lookup(testCtx, "testuser")
	if lookupErr == nil || lookupErr.Error() != expectedError {
		t.Errorf("TestLdapSearchUserNotFound: got unexpected error calling Lookup %v", lookupErr)
	}
}
