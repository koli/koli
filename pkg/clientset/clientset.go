package clientset

import (
	"fmt"
	"net/url"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// NewClusterConfig creates a new customized *kubernetes.Config
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

// NewSysRESTClient generates a new *kubernetes.Interface
// to communicate with Custom Resource Defintions resources
func NewSysRESTClient(c *rest.Config) (*CoreClient, error) {
	c.APIPath = "/apis"

	c.GroupVersion = &schema.GroupVersion{
		Group:   platform.GroupName,
		Version: platform.SchemeGroupVersion.Version,
	}
	contentConfig := dynamic.ContentConfig()
	contentConfig.GroupVersion = &platform.SchemeGroupVersion
	c.ContentConfig = contentConfig

	// c.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	cl, err := rest.RESTClientFor(c)
	if err != nil {
		return nil, err
	}
	return &CoreClient{restClient: cl}, nil
}

func NewKubernetesClient(c *rest.Config) (kubernetes.Interface, error) {
	var err error
	if len(c.Host) == 0 {
		c, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating client configuration: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
