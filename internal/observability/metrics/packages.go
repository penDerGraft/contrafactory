// Package metrics provides Prometheus instrumentation for contrafactory.
package metrics

// PackagePublish records a package publish operation.
func PackagePublish(chain, builder, status string) {
	if !enabled {
		return
	}
	packagePublishTotal.WithLabelValues(chain, builder, status).Inc()
}

// PackageRetrieve records a package retrieval operation.
func PackageRetrieve(status string) {
	if !enabled {
		return
	}
	packageRetrieveTotal.WithLabelValues(status).Inc()
}

// PackageDelete records a package deletion operation.
func PackageDelete(status string) {
	if !enabled {
		return
	}
	packageDeleteTotal.WithLabelValues(status).Inc()
}

// DeploymentRecord records a deployment record operation.
func DeploymentRecord(chain, status string) {
	if !enabled {
		return
	}
	deploymentRecordTotal.WithLabelValues(chain, status).Inc()
}

// DeploymentVerify records a deployment verification update.
func DeploymentVerify(status string) {
	if !enabled {
		return
	}
	deploymentVerifyTotal.WithLabelValues(status).Inc()
}

// VerificationRequest records a verification request.
func VerificationRequest(result string) {
	if !enabled {
		return
	}
	verificationTotal.WithLabelValues(result).Inc()
}
