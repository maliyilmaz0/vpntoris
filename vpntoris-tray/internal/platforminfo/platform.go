package platforminfo

type Capabilities struct {
	Platform          string `json:"platform"`
	StateDirectory    string `json:"stateDirectory"`
	InterfaceBackend  string `json:"interfaceBackend"`
	RouteBackend      string `json:"routeBackend"`
	DNSBackend        string `json:"dnsBackend"`
	CredentialBackend string `json:"credentialBackend"`
	PackageFormat     string `json:"packageFormat"`
}

func Current() Capabilities {
	return current()
}
