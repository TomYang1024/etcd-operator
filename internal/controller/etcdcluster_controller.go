/*
Copyright 2024 tomyang.

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

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etcdiov1alpha1 "github.com/yanghao89/etcd-operator/api/v1alpha1"
	"github.com/yanghao89/etcd-operator/internal/operator"
)

// EtcdClusterReconciler reconciles a EtcdCluster object
type EtcdClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EtcdCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *EtcdClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx)
	logger = logger.WithValues("etcdcluster", req.NamespacedName)

	var (
		etcdCluster etcdiov1alpha1.EtcdCluster
	)
	// 获取 EtcdCluster 实例
	if err := r.Get(ctx, req.NamespacedName, &etcdCluster); err != nil {
		return res, client.IgnoreNotFound(err)
	}
	// createOrUpdate
	var svc corev1.Service
	// 创建并更新 Headless Service
	svc.Name = etcdCluster.Name
	svc.Namespace = etcdCluster.Namespace
	_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = ctrl.CreateOrUpdate(ctx, r.Client, &svc, func() error {
			operator.MutateHeadlessSvc(&etcdCluster, &svc)
			return controllerutil.SetControllerReference(&etcdCluster, &svc, r.Scheme)
		})
		return err
	})
	logger.Info("create or update", "service", svc)
	var sts appsv1.StatefulSet
	sts.Name = etcdCluster.Name
	sts.Namespace = etcdCluster.Namespace
	_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = ctrl.CreateOrUpdate(ctx, r.Client, &sts, func() error {
			operator.MutateStatefulSet(&etcdCluster, &sts)
			return controllerutil.SetControllerReference(&etcdCluster, &sts, r.Scheme)
		})
		return err
	})

	logger.Info("create or update", "statefulset", sts)
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdiov1alpha1.EtcdCluster{}).
		For(&corev1.Service{}).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}
