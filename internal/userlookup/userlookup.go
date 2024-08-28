package userlookup

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"eric-odp-factory/internal/common"
	"eric-odp-factory/internal/config"

	ldap "github.com/go-ldap/ldap/v3"
)

const (
	usernameFile = "username"
	passwordFile = "password"

	ctxID = "ctxid"
)

type UserLookup interface {
	Lookup(ctx context.Context, username string) (*UserInfo, error)
}

type Production struct {
	ldapURL string

	tlsConfig *tls.Config

	ldapUserBaseDN string
	ldapUserQuery  string
	ldapUserAttrs  []string

	ldapGroupBaseDN string
	ldapGroupQuery  string
	ldapGroupAttrs  []string

	ldapUsername string
	ldapPassword string

	testLdap ldap.Client
}

type UserInfo struct {
	Username string
	UserAttr map[string]string
	Groups   string
	MemberOf []string
}

var errInvalidSearchResult = errors.New("invalid LDAP search result")

func NewUserLookup(cfg *config.Config) (*Production, error) {
	// for now, if LdapCredsDir is empty, skip reading the username/password
	// so that we can get the service up and running
	// This also will be the case if we're allowed query without
	// binding at all which seems to be the case for ENM
	var ldapUsernameBytes, ldapPasswordBytes []byte
	if cfg.LdapCredsDir != "" {
		var err error
		ldapUsernameBytes, err = os.ReadFile(cfg.LdapCredsDir + "/" + usernameFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read username from file: %w", err)
		}
		ldapPasswordBytes, err = os.ReadFile(cfg.LdapCredsDir + "/" + passwordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read password from file: %w", err)
		}
	}

	var tlsConfig *tls.Config
	if cfg.LdapCAFile != "" {
		ldapCaBytes, err := os.ReadFile(cfg.LdapCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA from file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(ldapCaBytes)
		tlsConfig = &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS12,
		}
	}

	ul := Production{
		ldapURL:        cfg.LdapURL,
		tlsConfig:      tlsConfig,
		ldapUserBaseDN: cfg.LdapUserBaseDN,
		ldapUserQuery:  cfg.LdapUserQuery,
		ldapUserAttrs:  strings.Split(cfg.LdapUserAttrs, ","),

		ldapGroupBaseDN: cfg.LdapGroupBaseDN,
		ldapGroupQuery:  cfg.LdapGroupQuery,
		ldapGroupAttrs:  strings.Split(cfg.LdapGroupAttrs, ","),

		ldapUsername: string(ldapUsernameBytes),
		ldapPassword: string(ldapPasswordBytes),
	}

	setupMetrics()

	slog.Info(
		"userlookup.Lookup InitUserLookup",
		"ldapUserAttrs",
		ul.ldapUserAttrs,
		"ldapGroupAttrs",
		ul.ldapGroupAttrs,
	)

	return &ul, nil
}

func (ul *Production) Lookup(ctx context.Context, username string) (*UserInfo, error) {
	slog.Debug("userlookup.Lookup", common.CtxIDLabel, ctx.Value(common.CtxID), "username", username)
	tStart := time.Now()
	ldapClient, err := ul.getLdapClient()
	tDial := time.Now()
	if err != nil {
		recordLdapError("dial")

		return nil, fmt.Errorf("failed to connected to LDAP: %w", err)
	}
	defer ldapClient.Close()

	recordLdapRequest("dial", tDial.Sub(tStart).Seconds())

	tBind := tDial
	if ul.ldapUsername != "" {
		err = ldapClient.Bind(ul.ldapUsername, ul.ldapPassword)
		tBind = time.Now()
		recordLdapRequest("bind", tBind.Sub(tDial).Seconds())
		if err != nil {
			recordLdapError("bind")

			return nil, fmt.Errorf("failed to bind to LDAP: %w", err)
		}
	}

	userInfo := UserInfo{Username: username}

	if err := ul.userQuery(ctx, username, ldapClient, &userInfo); err != nil {
		return nil, err
	}
	tUserSearch := time.Now()
	recordLdapRequest("search_user", tUserSearch.Sub(tBind).Seconds())

	if err := ul.groupsQuery(ctx, username, ldapClient, &userInfo); err != nil {
		return nil, err
	}
	tGroupSearch := time.Now()
	recordLdapRequest("search_group", tGroupSearch.Sub(tUserSearch).Seconds())

	return &userInfo, nil
}

func (ul *Production) userQuery(
	ctx context.Context,
	username string,
	ldapClient ldap.Client,
	userInfo *UserInfo,
) error {
	ldapUserQuery := fmt.Sprintf(ul.ldapUserQuery, username)
	searchUserRequest := ldap.NewSearchRequest(
		ul.ldapUserBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, // SizeLimit
		0, // TimeLimit
		false,
		ldapUserQuery, // The filter to apply
		ul.ldapUserAttrs,
		nil,
	)

	slog.Debug("userlookup.userQuery calling user search", common.CtxIDLabel, ctx.Value(common.CtxID),
		"ldapUserQuery", ldapUserQuery)
	searchUserResult, err := ldapClient.Search(searchUserRequest)
	if err != nil {
		recordLdapError("search_user")

		return fmt.Errorf("failed search for user: %w", err)
	}
	slog.Debug("userlookup.userQuery user search returned", ctxID, ctx.Value(ctxID),
		"#entries", len(searchUserResult.Entries))

	if len(searchUserResult.Entries) != 1 {
		return fmt.Errorf(
			"invalid number of search results %d: %w",
			len(searchUserResult.Entries),
			errInvalidSearchResult,
		)
	}

	slog.Debug("userlookup.userQuery", "user entry", ctxID, ctx.Value(ctxID), searchUserResult.Entries[0])

	userAttrs := make(map[string]string)
	for _, attrName := range ul.ldapUserAttrs {
		userAttrs[attrName] = strings.Join(
			searchUserResult.Entries[0].GetAttributeValues(attrName),
			",",
		)
	}
	userInfo.UserAttr = userAttrs

	return nil
}

func (ul *Production) groupsQuery(
	ctx context.Context,
	username string,
	ldapClient ldap.Client,
	userInfo *UserInfo,
) error {
	ldapGroupQuery := fmt.Sprintf(ul.ldapGroupQuery, username)
	searchGroupRequest := ldap.NewSearchRequest(
		ul.ldapGroupBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, // SizeLimit
		0, // TimeLimit
		false,
		ldapGroupQuery, // The filter to apply
		ul.ldapGroupAttrs,
		nil,
	)
	slog.Debug("userlookup.Lookup calling group search", ctxID, ctx.Value(ctxID), "ldapGroupQuery", ldapGroupQuery)
	searchGroupResult, err := ldapClient.Search(searchGroupRequest)
	if err != nil {
		recordLdapError("search_group")

		return fmt.Errorf("failed search for user groups: %w", err)
	}
	slog.Debug("userlookup.Lookup group search returned", ctxID, ctx.Value(ctxID),
		"#entries", len(searchGroupResult.Entries))

	groups := make([]string, 0, len(searchGroupResult.Entries))
	memberOf := make([]string, 0, len(searchGroupResult.Entries))
	for _, groupEntry := range searchGroupResult.Entries {
		// Assumation/Requirement here that the group name is allows the first attribute
		memberOf = append(memberOf, groupEntry.GetAttributeValue(ul.ldapGroupAttrs[0]))
		groupAttrs := make([]string, 0, len(ul.ldapGroupAttrs))
		for _, attrName := range ul.ldapGroupAttrs {
			groupAttrs = append(groupAttrs, groupEntry.GetAttributeValue(attrName))
		}
		groups = append(groups, strings.Join(groupAttrs, ":"))
	}

	userInfo.Groups = strings.Join(groups, ";")
	userInfo.MemberOf = memberOf

	return nil
}

func (ul *Production) getLdapClient() (ldap.Client, error) {
	if ul.testLdap != nil {
		return ul.testLdap, nil
	}

	ldapDialOpt := make([]ldap.DialOpt, 0, 1)
	if ul.tlsConfig != nil {
		ldapDialOpt = append(ldapDialOpt, ldap.DialWithTLSConfig(ul.tlsConfig))
	}

	return ldap.DialURL(ul.ldapURL, ldapDialOpt...) //nolint:wrapcheck // Caller will wrap
}

type Test struct {
	User UserInfo
	Err  error
}

func (ul *Test) Lookup(_ context.Context, _ string) (*UserInfo, error) {
	return &ul.User, ul.Err
}
