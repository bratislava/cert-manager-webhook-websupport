package main

import (
	"os"
	"testing"

	"github.com/jetstack/cert-manager/test/acme/dns"
	"github.com/cert-manager/webhook-example/example"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	//TODO: Write test for websupport cert-manager
	solver := example.New("59351")
	fixture := dns.NewFixture(solver,
		dns.SetResolvedZone("example.com."),
		dns.SetManifestPath("testdata/my-custom-solver"),
		dns.SetDNSServer("127.0.0.1:59351"),
		dns.SetUseAuthoritative(false),
	)
	fixture.RunBasic(t)
	fixture.RunExtended(t)
}
