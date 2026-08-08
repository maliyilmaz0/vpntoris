package nativeengine

import "testing"

func TestBuildNetworkPlanRejectsDefaultRoute(t *testing.T) {
	_, err := BuildNetworkPlan(ProfileNetwork{Profile: "office", Interface: "utun12", ProcessID: 42, Routes: []string{"0.0.0.0/0"}})
	if err == nil {
		t.Fatal("default route was accepted")
	}
}
func TestBuildNetworkPlanNormalizesAndOrdersRoutes(t *testing.T) {
	plan, err := BuildNetworkPlan(ProfileNetwork{Profile: "office", Interface: "utun12", ProcessID: 42, Routes: []string{"10.0.0.8/8", "10.0.0.1/32", "10.0.0.0/8"}, Domains: []string{"Corp.Example.com"}, DNSServers: []string{"10.0.0.53"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Mutations) != 5 {
		t.Fatalf("unexpected mutation count: %d", len(plan.Mutations))
	}
	if plan.Mutations[2].CIDR != "10.0.0.1/32" || plan.Mutations[3].CIDR != "10.0.0.0/8" {
		t.Fatalf("routes were not normalized and ordered: %#v", plan.Mutations)
	}
	if plan.Mutations[4].Domain != "corp.example.com" {
		t.Fatalf("domain was not normalized: %#v", plan.Mutations[4])
	}
}
func TestBuildNetworkPlanRequiresScopedDNS(t *testing.T) {
	_, err := BuildNetworkPlan(ProfileNetwork{Profile: "office", Interface: "utun12", ProcessID: 42, Routes: []string{"10.0.0.0/8"}, Domains: []string{"corp.example.com"}})
	if err == nil {
		t.Fatal("split DNS without servers was accepted")
	}
}
