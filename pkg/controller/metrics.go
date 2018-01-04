package controller

import "github.com/prometheus/client_golang/prometheus"

var (
	buildsCompleted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "koli_platform",
		Subsystem: "controller",
		Name:      "builds_completed",
		Help:      "Total number of build pod completed",
	})

	buildsDeployed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "koli_platform",
		Subsystem: "controller",
		Name:      "builds_deployed",
		Help:      "Total number of builds that are deployed after the build is completed",
	})

	buildsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "koli_platform",
		Subsystem: "controller",
		Name:      "builds_failed",
		Help:      "Total number of failed build",
	})

	pvcCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "koli_platform",
		Subsystem: "controller",
		Name:      "pvcs_created",
		Help:      "Total number of PVC's created'",
	})

	pvcFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "koli_platform",
		Subsystem: "controller",
		Name:      "pvcs_failed",
		Help:      "Total number of PVC's failed'",
	})
)

func init() {
	prometheus.MustRegister(buildsCompleted)
	prometheus.MustRegister(buildsDeployed)
	prometheus.MustRegister(buildsFailed)
	prometheus.MustRegister(pvcCreated)
	prometheus.MustRegister(pvcFailed)
}
