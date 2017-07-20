package receive

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/pkg/api/v1"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/git/k8s"
)

// waitForPod waits for a pod in state running, succeeded or failed
func waitForPod(pw *k8s.PodWatcher, ns, rev, buildRev string, ticker, interval, timeout time.Duration) error {
	condition := func(pod *v1.Pod) (bool, error) {
		if pod.Status.Phase == v1.PodRunning {
			return true, nil
		}
		if pod.Status.Phase == v1.PodSucceeded {
			return true, nil
		}
		if pod.Status.Phase == v1.PodFailed {
			return true, fmt.Errorf("Giving up; pod went into failed status: \n[%s]:%s", pod.Status.Reason, pod.Status.Message)
		}
		return false, nil
	}

	quit := progress("...", ticker)
	err := waitForPodCondition(pw, ns, rev, buildRev, condition, interval, timeout)
	quit <- true
	<-quit
	return err
}

// waitForPodEnd waits for a pod in state succeeded or failed
func waitForPodEnd(pw *k8s.PodWatcher, ns, rev, buildRev string, interval, timeout time.Duration) error {
	condition := func(pod *v1.Pod) (bool, error) {
		if pod.Status.Phase == v1.PodSucceeded {
			return true, nil
		}
		if pod.Status.Phase == v1.PodFailed {
			return true, nil
		}
		return false, nil
	}

	return waitForPodCondition(pw, ns, rev, buildRev, condition, interval, timeout)
}

// waitForPodCondition waits for a pod in state defined by a condition (func)
func waitForPodCondition(pw *k8s.PodWatcher, ns, rev, buildRev string, condition func(pod *v1.Pod) (bool, error),
	interval, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		pods, err := pw.Store.List(labels.Set{
			platform.AnnotationGitRevision:   rev,
			platform.AnnotationBuildRevision: buildRev,
		}.AsSelector())
		if err != nil || len(pods) == 0 {
			return false, nil
		}

		done, err := condition(pods[0])
		if err != nil {
			return false, err
		}
		if done {
			return true, nil
		}

		return false, nil
	})
}

func progress(msg string, interval time.Duration) chan bool {
	tick := time.Tick(interval)
	quit := make(chan bool)
	go func() {
		for {
			select {
			case <-quit:
				close(quit)
				return
			case <-tick:
				fmt.Println(msg)
			}
		}
	}()
	return quit
}
