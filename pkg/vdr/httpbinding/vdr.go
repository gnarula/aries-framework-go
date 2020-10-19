/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package httpbinding

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hyperledger/aries-framework-go/pkg/common/log"
	"github.com/hyperledger/aries-framework-go/pkg/doc/did"
	vdrapi "github.com/hyperledger/aries-framework-go/pkg/framework/aries/api/vdr"
)

var logger = log.New("aries-framework/vdr/httpbinding")

// VDR via HTTP(s) endpoint.
type VDR struct {
	endpointURL      string
	client           *http.Client
	accept           Accept
	resolveAuthToken string
}

// Accept is method to accept did method.
type Accept func(method string) bool

// New creates new DID Resolver.
func New(endpointURL string, opts ...Option) (*VDR, error) {
	v := &VDR{client: &http.Client{}, accept: func(method string) bool { return true }}

	for _, opt := range opts {
		opt(v)
	}

	// Validate host
	_, err := url.ParseRequestURI(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("base URL invalid: %w", err)
	}

	v.endpointURL = endpointURL

	return v, nil
}

// Accept did method - attempt to resolve any method.
func (v *VDR) Accept(method string) bool {
	return v.accept(method)
}

// Store did doc.
func (v *VDR) Store(doc *did.Doc, by *[]vdrapi.ModifiedBy) error {
	logger.Warnf(" store not supported in http binding vdr")
	return nil
}

// Build did doc.
func (v *VDR) Build(pubKey *vdrapi.PubKey, opts ...vdrapi.DocOpts) (*did.Doc, error) {
	return nil, fmt.Errorf("build not supported in http binding vdr")
}

// Close frees resources being maintained by vdr.
func (v *VDR) Close() error {
	return nil
}

// Option configures the peer vdr.
type Option func(opts *VDR)

// WithTimeout option is for definition of HTTP(s) timeout value of DID Resolver.
func WithTimeout(timeout time.Duration) Option {
	return func(opts *VDR) {
		opts.client.Timeout = timeout
	}
}

// WithTLSConfig option is for definition of secured HTTP transport using a tls.Config instance.
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(opts *VDR) {
		opts.client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}
}

// WithAccept option is for accept did method.
func WithAccept(accept Accept) Option {
	return func(opts *VDR) {
		opts.accept = accept
	}
}

// WithResolveAuthToken add auth token for resolve.
func WithResolveAuthToken(authToken string) Option {
	return func(opts *VDR) {
		opts.resolveAuthToken = "Bearer " + authToken
	}
}

func closeResponseBody(respBody io.Closer) {
	e := respBody.Close()
	if e != nil {
		logger.Errorf("Failed to close response body: %v", e)
	}
}
