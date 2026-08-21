package proxy

import (
	"net/http"
	"strings"
)

type providerTransportHTTPDoer struct {
	next            HTTPDoer
	transport       providerTransportDefinition
	credentialValue string
}

func newProviderTransportHTTPDoer(next HTTPDoer, provider providerDefinition, credentialValue string) HTTPDoer {
	return providerTransportHTTPDoer{
		next:            next,
		transport:       provider.activeTransport,
		credentialValue: strings.TrimSpace(credentialValue),
	}
}

func (doer providerTransportHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	authorizedRequest := request.Clone(request.Context())
	authentication := doer.transport.authentication
	authorizedRequest.Header.Set(authentication.Header, authentication.Prefix+doer.credentialValue)
	for _, header := range doer.transport.headers {
		authorizedRequest.Header.Set(header.Name, header.Value)
	}
	return doer.next.Do(authorizedRequest)
}
