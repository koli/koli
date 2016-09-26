package util

import (
	"fmt"
	"io"
	"time"

	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/kubectl"
)

// DeploymentColumns it's the columns of that shows on
var DeploymentColumns = []string{"NAME", "DESIRED", "CURRENT", "UP-TO-DATE", "AVAILABLE", "PAUSED", "AGE"}

func shortHumanDuration(d time.Duration) string {
	// Allow deviation no more than 2 seconds(excluded) to tolerate machine time
	// inconsistence, it can be considered as almost now.
	if seconds := int(d.Seconds()); seconds < -1 {
		return fmt.Sprintf("<invalid>")
	} else if seconds < 0 {
		return fmt.Sprintf("0s")
	} else if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if minutes := int(d.Minutes()); minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	} else if hours := int(d.Hours()); hours < 24 {
		return fmt.Sprintf("%dh", hours)
	} else if hours < 24*364 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

// translateTimestamp returns the elapsed time since timestamp in
// human-readable approximation.
func translateTimestamp(timestamp unversioned.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}
	return shortHumanDuration(time.Now().Sub(timestamp.Time))
}

// formatResourceName receives a resource kind, name, and boolean specifying
// whether or not to update the current name to "kind/name"
func formatResourceName(kind, name string, withKind bool) string {
	if !withKind || kind == "" {
		return name
	}

	return kind + "/" + name
}

// PrintDeployment it's a method to override printDeployment from kubectl.HumanReadablePrinter using the Handler function
// If you have a HumanReadablePrinter instance you can override the default printer format (addDefaultHandlers method)
//
// Example:
// printer.Handler(DeploymentColumns, PrintDeployment)
func PrintDeployment(deployment *extensions.Deployment, w io.Writer, options kubectl.PrintOptions) error {
	name := formatResourceName(options.Kind, deployment.Name, options.WithKind)

	if options.WithNamespace {
		if _, err := fmt.Fprintf(w, "%s\t", deployment.Namespace); err != nil {
			return err
		}
	}

	desiredReplicas := deployment.Spec.Replicas
	currentReplicas := deployment.Status.Replicas
	updatedReplicas := deployment.Status.UpdatedReplicas
	availableReplicas := deployment.Status.AvailableReplicas
	age := translateTimestamp(deployment.CreationTimestamp)
	paused := deployment.Spec.Paused
	if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%t\t%s", name, desiredReplicas, currentReplicas, updatedReplicas, availableReplicas, paused, age); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, kubectl.AppendLabels(deployment.Labels, options.ColumnLabels)); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, kubectl.AppendAllLabels(options.ShowLabels, deployment.Labels))
	return err
}
