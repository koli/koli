package controller

import (
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

func watch3PRs(host, endpoint string, kclient *kubernetes.Clientset) error {
	return wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
		resp, err := kclient.CoreClient.Client.Get(host + endpoint)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			return true, nil
		case http.StatusNotFound: // not set up yet. wait.
			return false, nil
		default:
			return false, fmt.Errorf("invalid status code: %v", resp.Status)
		}
	})
}
