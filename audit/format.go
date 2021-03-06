package audit

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SermoDigital/jose/jws"
	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/vault/helper/salt"
	"github.com/hashicorp/vault/logical"
	"github.com/mitchellh/copystructure"
)

type AuditFormatWriter interface {
	WriteRequest(io.Writer, *AuditRequestEntry) error
	WriteResponse(io.Writer, *AuditResponseEntry) error
	Salt() (*salt.Salt, error)
}

// AuditFormatter implements the Formatter interface, and allows the underlying
// marshaller to be swapped out
type AuditFormatter struct {
	AuditFormatWriter
}

func (f *AuditFormatter) FormatRequest(
	w io.Writer,
	config FormatterConfig,
	auth *logical.Auth,
	req *logical.Request,
	inErr error) error {

	if req == nil {
		return fmt.Errorf("request to request-audit a nil request")
	}

	if w == nil {
		return fmt.Errorf("writer for audit request is nil")
	}

	if f.AuditFormatWriter == nil {
		return fmt.Errorf("no format writer specified")
	}

	salt, err := f.Salt()
	if err != nil {
		return errwrap.Wrapf("error fetching salt: {{err}}", err)
	}

	if !config.Raw {
		// Before we copy the structure we must nil out some data
		// otherwise we will cause reflection to panic and die
		if req.Connection != nil && req.Connection.ConnState != nil {
			origReq := req
			origState := req.Connection.ConnState
			req.Connection.ConnState = nil
			defer func() {
				origReq.Connection.ConnState = origState
			}()
		}

		// Copy the auth structure
		if auth != nil {
			cp, err := copystructure.Copy(auth)
			if err != nil {
				return err
			}
			auth = cp.(*logical.Auth)
		}

		cp, err := copystructure.Copy(req)
		if err != nil {
			return err
		}
		req = cp.(*logical.Request)

		// Hash any sensitive information
		if auth != nil {
			// Cache and restore accessor in the auth
			var authAccessor string
			if !config.HMACAccessor && auth.Accessor != "" {
				authAccessor = auth.Accessor
			}
			if err := Hash(salt, auth); err != nil {
				return err
			}
			if authAccessor != "" {
				auth.Accessor = authAccessor
			}
		}

		// Cache and restore accessor in the request
		var clientTokenAccessor string
		if !config.HMACAccessor && req != nil && req.ClientTokenAccessor != "" {
			clientTokenAccessor = req.ClientTokenAccessor
		}
		if err := Hash(salt, req); err != nil {
			return err
		}
		if clientTokenAccessor != "" {
			req.ClientTokenAccessor = clientTokenAccessor
		}
	}

	// If auth is nil, make an empty one
	if auth == nil {
		auth = new(logical.Auth)
	}
	var errString string
	if inErr != nil {
		errString = inErr.Error()
	}

	reqEntry := &AuditRequestEntry{
		Type:  "request",
		Error: errString,

		Auth: AuditAuth{
			ClientToken:   auth.ClientToken,
			Accessor:      auth.Accessor,
			DisplayName:   auth.DisplayName,
			Policies:      auth.Policies,
			Metadata:      auth.Metadata,
			EntityID:      auth.EntityID,
			RemainingUses: req.ClientTokenRemainingUses,
		},

		Request: AuditRequest{
			ID:                  req.ID,
			ClientToken:         req.ClientToken,
			ClientTokenAccessor: req.ClientTokenAccessor,
			Operation:           req.Operation,
			Path:                req.Path,
			Data:                req.Data,
			PolicyOverride:      req.PolicyOverride,
			RemoteAddr:          getRemoteAddr(req),
			ReplicationCluster:  req.ReplicationCluster,
			Headers:             req.Headers,
		},
	}

	if req.WrapInfo != nil {
		reqEntry.Request.WrapTTL = int(req.WrapInfo.TTL / time.Second)
	}

	if !config.OmitTime {
		reqEntry.Time = time.Now().UTC().Format(time.RFC3339)
	}

	return f.AuditFormatWriter.WriteRequest(w, reqEntry)
}

func (f *AuditFormatter) FormatResponse(
	w io.Writer,
	config FormatterConfig,
	auth *logical.Auth,
	req *logical.Request,
	resp *logical.Response,
	inErr error) error {

	if req == nil {
		return fmt.Errorf("request to response-audit a nil request")
	}

	if w == nil {
		return fmt.Errorf("writer for audit request is nil")
	}

	if f.AuditFormatWriter == nil {
		return fmt.Errorf("no format writer specified")
	}

	salt, err := f.Salt()
	if err != nil {
		return errwrap.Wrapf("error fetching salt: {{err}}", err)
	}

	if !config.Raw {
		// Before we copy the structure we must nil out some data
		// otherwise we will cause reflection to panic and die
		if req.Connection != nil && req.Connection.ConnState != nil {
			origReq := req
			origState := req.Connection.ConnState
			req.Connection.ConnState = nil
			defer func() {
				origReq.Connection.ConnState = origState
			}()
		}

		// Copy the auth structure
		if auth != nil {
			cp, err := copystructure.Copy(auth)
			if err != nil {
				return err
			}
			auth = cp.(*logical.Auth)
		}

		cp, err := copystructure.Copy(req)
		if err != nil {
			return err
		}
		req = cp.(*logical.Request)

		if resp != nil {
			cp, err := copystructure.Copy(resp)
			if err != nil {
				return err
			}
			resp = cp.(*logical.Response)
		}

		// Hash any sensitive information

		// Cache and restore accessor in the auth
		if auth != nil {
			var accessor string
			if !config.HMACAccessor && auth.Accessor != "" {
				accessor = auth.Accessor
			}
			if err := Hash(salt, auth); err != nil {
				return err
			}
			if accessor != "" {
				auth.Accessor = accessor
			}
		}

		// Cache and restore accessor in the request
		var clientTokenAccessor string
		if !config.HMACAccessor && req != nil && req.ClientTokenAccessor != "" {
			clientTokenAccessor = req.ClientTokenAccessor
		}
		if err := Hash(salt, req); err != nil {
			return err
		}
		if clientTokenAccessor != "" {
			req.ClientTokenAccessor = clientTokenAccessor
		}

		// Cache and restore accessor in the response
		if resp != nil {
			var accessor, wrappedAccessor string
			if !config.HMACAccessor && resp != nil && resp.Auth != nil && resp.Auth.Accessor != "" {
				accessor = resp.Auth.Accessor
			}
			if !config.HMACAccessor && resp != nil && resp.WrapInfo != nil && resp.WrapInfo.WrappedAccessor != "" {
				wrappedAccessor = resp.WrapInfo.WrappedAccessor
			}
			if err := Hash(salt, resp); err != nil {
				return err
			}
			if accessor != "" {
				resp.Auth.Accessor = accessor
			}
			if wrappedAccessor != "" {
				resp.WrapInfo.WrappedAccessor = wrappedAccessor
			}
		}
	}

	// If things are nil, make empty to avoid panics
	if auth == nil {
		auth = new(logical.Auth)
	}
	if resp == nil {
		resp = new(logical.Response)
	}
	var errString string
	if inErr != nil {
		errString = inErr.Error()
	}

	var respAuth *AuditAuth
	if resp.Auth != nil {
		respAuth = &AuditAuth{
			ClientToken: resp.Auth.ClientToken,
			Accessor:    resp.Auth.Accessor,
			DisplayName: resp.Auth.DisplayName,
			Policies:    resp.Auth.Policies,
			Metadata:    resp.Auth.Metadata,
			NumUses:     resp.Auth.NumUses,
		}
	}

	var respSecret *AuditSecret
	if resp.Secret != nil {
		respSecret = &AuditSecret{
			LeaseID: resp.Secret.LeaseID,
		}
	}

	var respWrapInfo *AuditResponseWrapInfo
	if resp.WrapInfo != nil {
		token := resp.WrapInfo.Token
		if jwtToken := parseVaultTokenFromJWT(token); jwtToken != nil {
			token = *jwtToken
		}
		respWrapInfo = &AuditResponseWrapInfo{
			TTL:             int(resp.WrapInfo.TTL / time.Second),
			Token:           token,
			CreationTime:    resp.WrapInfo.CreationTime.Format(time.RFC3339Nano),
			CreationPath:    resp.WrapInfo.CreationPath,
			WrappedAccessor: resp.WrapInfo.WrappedAccessor,
		}
	}

	respEntry := &AuditResponseEntry{
		Type:  "response",
		Error: errString,
		Auth: AuditAuth{
			DisplayName:   auth.DisplayName,
			Policies:      auth.Policies,
			Metadata:      auth.Metadata,
			ClientToken:   auth.ClientToken,
			Accessor:      auth.Accessor,
			RemainingUses: req.ClientTokenRemainingUses,
			EntityID:      auth.EntityID,
		},

		Request: AuditRequest{
			ID:                  req.ID,
			ClientToken:         req.ClientToken,
			ClientTokenAccessor: req.ClientTokenAccessor,
			Operation:           req.Operation,
			Path:                req.Path,
			Data:                req.Data,
			PolicyOverride:      req.PolicyOverride,
			RemoteAddr:          getRemoteAddr(req),
			ReplicationCluster:  req.ReplicationCluster,
			Headers:             req.Headers,
		},

		Response: AuditResponse{
			Auth:     respAuth,
			Secret:   respSecret,
			Data:     resp.Data,
			Redirect: resp.Redirect,
			WrapInfo: respWrapInfo,
		},
	}

	if req.WrapInfo != nil {
		respEntry.Request.WrapTTL = int(req.WrapInfo.TTL / time.Second)
	}

	if !config.OmitTime {
		respEntry.Time = time.Now().UTC().Format(time.RFC3339)
	}

	return f.AuditFormatWriter.WriteResponse(w, respEntry)
}

// AuditRequest is the structure of a request audit log entry in Audit.
type AuditRequestEntry struct {
	Time    string       `json:"time,omitempty"`
	Type    string       `json:"type"`
	Auth    AuditAuth    `json:"auth"`
	Request AuditRequest `json:"request"`
	Error   string       `json:"error"`
}

// AuditResponseEntry is the structure of a response audit log entry in Audit.
type AuditResponseEntry struct {
	Time     string        `json:"time,omitempty"`
	Type     string        `json:"type"`
	Auth     AuditAuth     `json:"auth"`
	Request  AuditRequest  `json:"request"`
	Response AuditResponse `json:"response"`
	Error    string        `json:"error"`
}

type AuditRequest struct {
	ID                  string                 `json:"id"`
	ReplicationCluster  string                 `json:"replication_cluster,omitempty"`
	Operation           logical.Operation      `json:"operation"`
	ClientToken         string                 `json:"client_token"`
	ClientTokenAccessor string                 `json:"client_token_accessor"`
	Path                string                 `json:"path"`
	Data                map[string]interface{} `json:"data"`
	PolicyOverride      bool                   `json:"policy_override"`
	RemoteAddr          string                 `json:"remote_address"`
	WrapTTL             int                    `json:"wrap_ttl"`
	Headers             map[string][]string    `json:"headers"`
}

type AuditResponse struct {
	Auth     *AuditAuth             `json:"auth,omitempty"`
	Secret   *AuditSecret           `json:"secret,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Redirect string                 `json:"redirect,omitempty"`
	WrapInfo *AuditResponseWrapInfo `json:"wrap_info,omitempty"`
}

type AuditAuth struct {
	ClientToken   string            `json:"client_token"`
	Accessor      string            `json:"accessor"`
	DisplayName   string            `json:"display_name"`
	Policies      []string          `json:"policies"`
	Metadata      map[string]string `json:"metadata"`
	NumUses       int               `json:"num_uses,omitempty"`
	RemainingUses int               `json:"remaining_uses,omitempty"`
	EntityID      string            `json:"entity_id"`
}

type AuditSecret struct {
	LeaseID string `json:"lease_id"`
}

type AuditResponseWrapInfo struct {
	TTL             int    `json:"ttl"`
	Token           string `json:"token"`
	CreationTime    string `json:"creation_time"`
	CreationPath    string `json:"creation_path"`
	WrappedAccessor string `json:"wrapped_accessor,omitempty"`
}

// getRemoteAddr safely gets the remote address avoiding a nil pointer
func getRemoteAddr(req *logical.Request) string {
	if req != nil && req.Connection != nil {
		return req.Connection.RemoteAddr
	}
	return ""
}

// parseVaultTokenFromJWT returns a string iff the token was a JWT and we could
// extract the original token ID from inside
func parseVaultTokenFromJWT(token string) *string {
	if strings.Count(token, ".") != 2 {
		return nil
	}

	wt, err := jws.ParseJWT([]byte(token))
	if err != nil || wt == nil {
		return nil
	}

	result, _ := wt.Claims().JWTID()

	return &result
}
