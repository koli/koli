package clientset

import (
	"fmt"
	"net/url"

	"github.com/kolibox/koli/pkg/spec"

	"k8s.io/client-go/1.5/dynamic"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/rest"
)

// Interface core
type Interface interface {
	Core() CoreInterface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*CoreClient
}

// Core retrieves the CoreClient
func (c *Clientset) Core() CoreInterface {
	if c == nil {
		return nil
	}
	return c.CoreClient
}

// NewClusterConfig creates a new customized *rest.Config
func NewClusterConfig(host string, tlsInsecure bool, tlsConfig *rest.TLSClientConfig) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	if len(host) == 0 {
		if cfg, err = rest.InClusterConfig(); err != nil {
			return nil, err
		}
	} else {
		cfg = &rest.Config{
			Host: host,
		}
		hostURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("error parsing host url %s : %v", host, err)
		}
		if hostURL.Scheme == "https" {
			cfg.TLSClientConfig = *tlsConfig
			cfg.Insecure = tlsInsecure
		}
	}
	cfg.QPS = 100
	cfg.Burst = 100

	return cfg, nil
}

// NewSysRESTClient generates a new *rest.RESTClient to communicate with system third party resources
func NewSysRESTClient(c *rest.Config) (*CoreClient, error) {
	c.APIPath = "/apis"

	c.GroupVersion = &unversioned.GroupVersion{
		Group:   spec.GroupName,
		Version: spec.SchemeGroupVersion.Version,
	}
	contentConfig := dynamic.ContentConfig()
	contentConfig.GroupVersion = &spec.SchemeGroupVersion
	c.ContentConfig = contentConfig

	// c.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	cl, err := rest.RESTClientFor(c)
	if err != nil {
		return nil, err
	}
	return &CoreClient{restClient: cl}, nil
}