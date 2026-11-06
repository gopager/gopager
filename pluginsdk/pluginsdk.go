// Package pluginsdk is the Go SDK for GoPager plugins (plugin-runtime 2.0).
//
// A plugin is an executable that the GoPager Core starts as a child process through HashiCorp
// go-plugin and calls over gRPC. The wire contract is api/plugin/v2/plugin.proto: one Invoke
// method whose request and response are google.protobuf.BytesValue messages carrying UTF-8 JSON.
// Plugins in other languages implement that proto plus the go-plugin handshake directly.
//
// A minimal plugin:
//
//	func main() {
//		pluginsdk.Serve(pluginsdk.HandlerFunc(func(ctx context.Context, request []byte) ([]byte, error) {
//			mode, err := pluginsdk.RequestMode(request)
//			if err != nil {
//				return nil, err
//			}
//			_ = mode // pluginsdk.ModePreview only reaches plugins that declare supportsPreview
//			return []byte(`{"schemaVersion":2,"fields":{}}`), nil
//		}))
//	}
package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// ProtocolVersion is the go-plugin application protocol version of plugin-runtime 2.0.
	ProtocolVersion = 2
	// MagicCookieKey is the environment variable the Core sets when starting a
	// plugin; a plugin refuses to run when it is missing or wrong.
	MagicCookieKey = "GOPAGER_PLUGIN"
	// MagicCookieValue is the value expected in MagicCookieKey.
	MagicCookieValue = "gopager-plugin-runtime-2"
	// PluginName is the go-plugin dispense name of the single invocation service.
	PluginName = "gopager"
	// ServiceName is the fully qualified gRPC service name published in plugin.proto.
	ServiceName = "gopager.plugin.v2.Plugin"
	// InvokeMethod is the full gRPC method path of the Invoke call.
	InvokeMethod = "/" + ServiceName + "/Invoke"
)

// Handshake is shared by the Core and plugins; a mismatch fails the plugin start explicitly.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   MagicCookieKey,
	MagicCookieValue: MagicCookieValue,
}

// Invocation modes carried in the request's "mode" property.
const (
	// ModeLive is an ordinary processing run and the default when "mode" is absent.
	ModeLive = "live"
	// ModePreview is a dry run for an editor: the plugin must compute its result without any
	// effect outside its process. The Core sends it only to plugins whose manifest sets
	// "supportsPreview", so a plugin without that declaration never needs to handle it.
	ModePreview = "preview"
)

// RequestMode returns the invocation mode of a request: ModeLive when the property is absent,
// ModePreview or ModeLive otherwise, and an error for malformed JSON or an unknown mode.
func RequestMode(request []byte) (string, error) {
	var header struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(request, &header); err != nil {
		return "", fmt.Errorf("pluginsdk: invalid request: %w", err)
	}
	switch header.Mode {
	case "", ModeLive:
		return ModeLive, nil
	case ModePreview:
		return ModePreview, nil
	default:
		return "", fmt.Errorf("pluginsdk: unknown invocation mode %q", header.Mode)
	}
}

// Handler receives one JSON request and returns one JSON response.
// The context ends when the Core's deadline expires or the call is cancelled. The Core keeps one
// long-lived process per configured instance and sends it one request at a time, so a Handler
// may keep state between calls without locking but must not rely on a fresh process per call.
type Handler interface {
	Invoke(ctx context.Context, request []byte) ([]byte, error)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, request []byte) ([]byte, error)

// Invoke calls f.
func (f HandlerFunc) Invoke(ctx context.Context, request []byte) ([]byte, error) {
	return f(ctx, request)
}

// Serve runs the plugin until the Core stops it. It does not return under normal operation.
func Serve(handler Handler) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         plugin.PluginSet{PluginName: &GRPCPlugin{Handler: handler}},
		GRPCServer:      plugin.DefaultGRPCServer,
	})
}

// GRPCPlugin binds the invocation service to go-plugin on both sides.
type GRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	// Handler is required when serving; the Core leaves it nil.
	Handler Handler
}

// GRPCServer registers the plugin's Handler.
func (p *GRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, server *grpc.Server) error {
	if p.Handler == nil {
		return errors.New("pluginsdk: missing handler")
	}
	server.RegisterService(&serviceDesc, p.Handler)
	return nil
}

// GRPCClient returns the Core-side Client.
func (p *GRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	return &Client{conn: conn}, nil
}

// Client is the Core-side invocation stub.
type Client struct{ conn *grpc.ClientConn }

// Invoke sends request and accepts at most maxResponse response bytes.
func (c *Client) Invoke(ctx context.Context, request []byte, maxResponse int) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("pluginsdk: missing connection")
	}
	response := new(wrapperspb.BytesValue)
	// The BytesValue framing adds a few bytes on top of the JSON payload.
	if err := c.conn.Invoke(ctx, InvokeMethod, wrapperspb.Bytes(request), response,
		grpc.MaxCallRecvMsgSize(maxResponse+16)); err != nil {
		return nil, err
	}
	return response.GetValue(), nil
}

// serviceDesc is written by hand on the well-known BytesValue type, so no generated code is needed.
var serviceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*Handler)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Invoke",
		Handler: func(server any, ctx context.Context, decode func(any) error,
			interceptor grpc.UnaryServerInterceptor) (any, error) {
			request := new(wrapperspb.BytesValue)
			if err := decode(request); err != nil {
				return nil, err
			}
			call := func(ctx context.Context, value any) (any, error) {
				response, err := server.(Handler).Invoke(ctx, value.(*wrapperspb.BytesValue).GetValue())
				if err != nil {
					return nil, err
				}
				return wrapperspb.Bytes(response), nil
			}
			if interceptor == nil {
				return call(ctx, request)
			}
			return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: server, FullMethod: InvokeMethod}, call)
		},
	}},
	Metadata: "api/plugin/v2/plugin.proto",
}
