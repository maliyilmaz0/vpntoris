package netbackend

type Router interface {
	AddRoutes(interfaceName string, routes []string) error
	DeleteRoutes(interfaceName string, routes []string)
}
