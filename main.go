package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	logf "github.com/cert-manager/cert-manager/pkg/logs"

	"github.com/bratislava/cert-manager-webhook-websupport/websupport"
)

var log = logf.Log.WithName("websupport-solver")
var GroupName = os.Getenv("GROUP_NAME")

func splitDomain(domain string) (string, string) {
	parts := strings.Split(strings.TrimSuffix(domain, "."), ".")
	lens := len(parts)
	return strings.Join(parts[lens-2:], "."), strings.Join(parts[:lens-2], ".")
}

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&webSupportDNSProviderSolver{},
	)
}

// customDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type webSupportDNSProviderSolver struct {
	// If a Kubernetes 'clientset' is needed, you must:
	// 1. uncomment the additional `client` field in this structure below
	// 2. uncomment the "k8s.io/client-go/kubernetes" import at the top of the file
	// 3. uncomment the relevant code in the Initialize method below
	// 4. ensure your webhook's service account has the required RBAC role
	//    assigned to it for interacting with the Kubernetes APIs you need.
	client *kubernetes.Clientset
}

// customDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type webSupportDNSProviderConfig struct {
	// Change the two fields below according to the format of the configuration
	// to be decoded.
	// These fields will be set by users in the
	// `issuer.spec.acme.dns01.providers.webhook.config` field.

	Email           string                     `json:"email"`
	APIKeySecretRef cmmetav1.SecretKeySelector `json:"apiKeySecretRef"`
	ApiKey          string                     `json:"ApiKey"`
	ApiSecret       string                     `json:"ApiSecret"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *webSupportDNSProviderSolver) Name() string {
	return "websupport-solver"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *webSupportDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := c.loadConfig(ch)
	if err != nil {
		return err
	}

	client := websupport.NewClient(&websupport.Config{
		ApiKey:    cfg.ApiKey,
		ApiSecret: cfg.ApiSecret,
	})

	log.Info(fmt.Sprintf("Attempting to create record for '%s' with content '%s'", ch.ResolvedFQDN, string(ch.Key)))
	baseDomain, subdomain := splitDomain(ch.ResolvedFQDN)
	err = client.CreateRecord(baseDomain, &websupport.DnsRecord{
		Type:    "TXT",
		Name:    subdomain,
		Content: string(ch.Key),
		Ttl:     600,
	})
	if err != nil && errors.As(err, &websupport.WebsupportError{}) {
		// Most likely the record already exists. Just skip as we should
		// tolerate being called multiple times
		return nil
	}
	return err
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *webSupportDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := c.loadConfig(ch)
	if err != nil {
		return err
	}

	client := websupport.NewClient(&websupport.Config{
		ApiKey:    cfg.ApiKey,
		ApiSecret: cfg.ApiSecret,
	})

	log.Info(fmt.Sprintf("Attempting to delete record '%s' with content '%s'", ch.ResolvedFQDN, string(ch.Key)))
	baseDomain, subdomain := splitDomain(ch.ResolvedFQDN)
	return client.DeleteRecord(baseDomain, &websupport.DnsRecord{
		Type:    "TXT",
		Name:    subdomain,
		Content: string(ch.Key),
		Ttl:     600,
	})
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initializing
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *webSupportDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}
	c.client = cl
	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func (c *webSupportDNSProviderSolver) loadConfig(ch *v1alpha1.ChallengeRequest) (webSupportDNSProviderConfig, error) {
	cfg := webSupportDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if ch.Config == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(ch.Config.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	// try fill from secret if defined
	if cfg.APIKeySecretRef.Name != "" {
		namespace := ch.ResourceNamespace
		secretName := cfg.APIKeySecretRef.Name

		secret, err := c.client.CoreV1().Secrets(namespace).Get(
			context.TODO(),
			secretName,
			metav1.GetOptions{},
		)
		if err != nil {
			return cfg, fmt.Errorf("failed to load secret %s/%s: %w", namespace, secretName, err)
		}

		cfg.ApiKey = string(secret.Data["ApiKey"])
		cfg.ApiSecret = string(secret.Data["ApiSecret"])
	}

	return cfg, nil
}
