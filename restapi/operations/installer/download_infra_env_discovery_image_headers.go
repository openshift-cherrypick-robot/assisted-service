// Code generated by go-swagger; DO NOT EDIT.

package installer

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// DownloadInfraEnvDiscoveryImageHeadersHandlerFunc turns a function with the right signature into a download infra env discovery image headers handler
type DownloadInfraEnvDiscoveryImageHeadersHandlerFunc func(DownloadInfraEnvDiscoveryImageHeadersParams, interface{}) middleware.Responder

// Handle executing the request and returning a response
func (fn DownloadInfraEnvDiscoveryImageHeadersHandlerFunc) Handle(params DownloadInfraEnvDiscoveryImageHeadersParams, principal interface{}) middleware.Responder {
	return fn(params, principal)
}

// DownloadInfraEnvDiscoveryImageHeadersHandler interface for that can handle valid download infra env discovery image headers params
type DownloadInfraEnvDiscoveryImageHeadersHandler interface {
	Handle(DownloadInfraEnvDiscoveryImageHeadersParams, interface{}) middleware.Responder
}

// NewDownloadInfraEnvDiscoveryImageHeaders creates a new http.Handler for the download infra env discovery image headers operation
func NewDownloadInfraEnvDiscoveryImageHeaders(ctx *middleware.Context, handler DownloadInfraEnvDiscoveryImageHeadersHandler) *DownloadInfraEnvDiscoveryImageHeaders {
	return &DownloadInfraEnvDiscoveryImageHeaders{Context: ctx, Handler: handler}
}

/*DownloadInfraEnvDiscoveryImageHeaders swagger:route HEAD /v2/infra-envs/{infra_env_id}/downloads/image installer downloadInfraEnvDiscoveryImageHeaders

Downloads the discovery image Headers only.

*/
type DownloadInfraEnvDiscoveryImageHeaders struct {
	Context *middleware.Context
	Handler DownloadInfraEnvDiscoveryImageHeadersHandler
}

func (o *DownloadInfraEnvDiscoveryImageHeaders) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		r = rCtx
	}
	var Params = NewDownloadInfraEnvDiscoveryImageHeadersParams()

	uprinc, aCtx, err := o.Context.Authorize(r, route)
	if err != nil {
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}
	if aCtx != nil {
		r = aCtx
	}
	var principal interface{}
	if uprinc != nil {
		principal = uprinc
	}

	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params, principal) // actually handle the request

	o.Context.Respond(rw, r, route.Produces, route, res)

}
