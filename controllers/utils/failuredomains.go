/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	infrav1 "sigs.k8s.io/cluster-api-provider-cloudstack/api/v1beta2"
	"sigs.k8s.io/cluster-api-provider-cloudstack/pkg/cloud"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CreateFailureDomain creates a specified CloudStackFailureDomain CRD owned by the ReconcilationSubject.
func (r *ReconciliationRunner) CreateFailureDomain(fdSpec infrav1.CloudStackFailureDomainSpec) error {
	csFD := &infrav1.CloudStackFailureDomain{
		ObjectMeta: r.NewChildObjectMeta(fdSpec.Name),
		Spec:       fdSpec,
	}
	return errors.Wrap(r.K8sClient.Create(r.RequestCtx, csFD), "creating CloudStackFailureDomain")
}

// CreateFailureDomains creates a CloudStackFailureDomain CRD for each of the ReconcilationSubject's FailureDomains.
func (r *ReconciliationRunner) CreateFailureDomains(fdSpecs []infrav1.CloudStackFailureDomainSpec) CloudStackReconcilerMethod {
	return func() (ctrl.Result, error) {
		for _, fdSpec := range fdSpecs {
			if err := r.CreateFailureDomain(fdSpec); err != nil {
				if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
					return reconcile.Result{}, errors.Wrap(err, "creating CloudStackFailureDomains")
				}
			}
		}
		return ctrl.Result{}, nil
	}
}

// GetFailureDomains gets CloudStackFailureDomains owned by a CloudStackCluster.
func (r *ReconciliationRunner) GetFailureDomains(fds *infrav1.CloudStackFailureDomainList) CloudStackReconcilerMethod {
	return func() (ctrl.Result, error) {
		capiClusterLabel := map[string]string{clusterv1.ClusterLabelName: r.CSCluster.GetLabels()[clusterv1.ClusterLabelName]}
		if err := r.K8sClient.List(
			r.RequestCtx,
			fds,
			client.InNamespace(r.Request.Namespace),
			client.MatchingLabels(capiClusterLabel),
		); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to list failure domains")
		}
		return ctrl.Result{}, nil
	}
}

// GetFailureDomainssAndRequeueIfMissing gets CloudStackFailureDomains owned by a CloudStackCluster and requeues if none are found.
func (r *ReconciliationRunner) GetFailureDomainssAndRequeueIfMissing(fds *infrav1.CloudStackFailureDomainList) CloudStackReconcilerMethod {
	return func() (ctrl.Result, error) {
		if res, err := r.GetFailureDomains(fds)(); r.ShouldReturn(res, err) {
			return res, err
		} else if len(fds.Items) < 1 {
			return r.RequeueWithMessage("no failure domains found, requeueing")
		}
		return ctrl.Result{}, nil
	}
}

// AsFailureDomainUser uses the credentials specified in the failure domain to set the ReconciliationSubject's CSUser client.
func (r *ReconciliationRunner) AsFailureDomainUser(fdSpec infrav1.CloudStackFailureDomainSpec) CloudStackReconcilerMethod {
	return func() (ctrl.Result, error) {
		endpointCredentials := &corev1.Secret{}
		key := client.ObjectKey{Name: fdSpec.ACSEndpoint.Name, Namespace: fdSpec.ACSEndpoint.Namespace}
		if err := r.K8sClient.Get(r.RequestCtx, key, endpointCredentials); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "getting ACSEndpoint secret with ref: %v", fdSpec.ACSEndpoint)
		}


		r.CSUser = r.CSClient
		cloud.NewClient()
		if r.CSCluster.Spec.Account != "" {
			user := &cloud.User{}
			user.Account.Domain.Path = r.CSCluster.Spec.Domain
			user.Account.Name = r.CSCluster.Spec.Account
			if found, err := r.CSClient.GetUserWithKeys(user); err != nil {
				return ctrl.Result{}, err
			} else if !found {
				return ctrl.Result{}, errors.Errorf("could not find sufficient user (with API keys) in domain/account %s/%s",
					r.CSCluster.Spec.Domain, r.CSCluster.Spec.Account)
			}
			cfg := cloud.Config{APIKey: user.APIKey, SecretKey: user.SecretKey}
			client, err := r.CSClient.NewClientFromSpec(cfg)
			if err != nil {
				return ctrl.Result{}, err
			}
			r.CSUser = client
		}

		return ctrl.Result{}, nil
	}
}

		endpointCredentials.StringData["api-key"]
		endpointCredentials.StringData["api-url"]
		endpointCredentials.StringData["secret-key"]
		endpointCredentials.StringData["verify-ssl"]
