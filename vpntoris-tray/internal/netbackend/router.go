package netbackend

// Router applies and removes destination routes owned by a VPN session.
// Implementations must reject or never install default routes; callers still
// validate CIDRs before invoking the backend.
type Router interface {
	AddRoutes(interfaceName string, routes []string) error
	DeleteRoutes(interfaceName string, routes []string)
}
