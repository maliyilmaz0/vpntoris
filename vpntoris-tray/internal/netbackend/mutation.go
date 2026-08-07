package netbackend

import (
	"context"
	"fmt"
	"strings"

	"vpntoris-tray/internal/nativeengine"
)

// MutationBackend adapts a Router to nativeengine.Backend for route mutations.
// Interface, process and DNS mutations are reserved for later platform work.
type MutationBackend struct {
	Router Router
	DNS    DNSConfigurator
}

func (backend MutationBackend) Apply(ctx context.Context, mutation nativeengine.Mutation) error {
	_ = ctx
	switch mutation.Kind {
	case nativeengine.MutationRoute:
		if backend.Router == nil {
			return fmt.Errorf("route backend is required")
		}
		if mutation.Interface == "" || mutation.CIDR == "" {
			return fmt.Errorf("route mutation requires interface and cidr")
		}
		return backend.Router.AddRoutes(mutation.Interface, []string{mutation.CIDR})
	case nativeengine.MutationDNS:
		if backend.DNS == nil {
			return fmt.Errorf("dns backend is required")
		}
		if mutation.Interface == "" || mutation.Domain == "" {
			return fmt.Errorf("dns mutation requires interface and domain")
		}
		servers := []string{}
		if mutation.Values != nil && mutation.Values["servers"] != "" {
			for _, part := range splitCSV(mutation.Values["servers"]) {
				servers = append(servers, part)
			}
		}
		return backend.DNS.AddScoped(mutation.Interface, mutation.Domain, servers)
	case nativeengine.MutationInterface, nativeengine.MutationProcess:
		return nil
	default:
		return fmt.Errorf("unsupported mutation kind: %s", mutation.Kind)
	}
}

func (backend MutationBackend) Undo(ctx context.Context, mutation nativeengine.Mutation) error {
	_ = ctx
	switch mutation.Kind {
	case nativeengine.MutationRoute:
		if backend.Router == nil {
			return fmt.Errorf("route backend is required")
		}
		if mutation.Interface == "" || mutation.CIDR == "" {
			return nil
		}
		backend.Router.DeleteRoutes(mutation.Interface, []string{mutation.CIDR})
		return nil
	case nativeengine.MutationDNS:
		if backend.DNS == nil || mutation.Interface == "" || mutation.Domain == "" {
			return nil
		}
		return backend.DNS.RemoveScoped(mutation.Interface, mutation.Domain)
	default:
		return nil
	}
}

func (backend MutationBackend) Owned(ctx context.Context, mutation nativeengine.Mutation) (bool, error) {
	_ = ctx
	switch mutation.Kind {
	case nativeengine.MutationRoute:
		return mutation.Interface != "" && mutation.CIDR != "", nil
	case nativeengine.MutationDNS:
		return mutation.Interface != "" && mutation.Domain != "", nil
	case nativeengine.MutationInterface, nativeengine.MutationProcess:
		return false, nil
	default:
		return false, nil
	}
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
