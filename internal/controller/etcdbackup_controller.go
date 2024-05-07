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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"etcd-operator/api/v1alpha1"

	etcdiov1alpha1 "etcd-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdBackupReconciler reconciles a EtcdBackup object
type EtcdBackupReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	BackupImage string
}

type backupState struct {
	backup  *v1alpha1.EtcdBackup
	actual  *backupStateContainer
	desired *backupStateContainer
}

func (r *EtcdBackupReconciler) getState(ctx context.Context, req ctrl.Request) (state *backupState, err error) {
	state = &backupState{
		backup: &v1alpha1.EtcdBackup{},
	}
	// get
	if err = r.Get(ctx, req.NamespacedName, state.backup); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.New(fmt.Sprintf("failed to get backup: %s", err.Error()))
		}
		state.backup = nil
		return state, nil
	}
	// 获取真实的状态
	if err = r.setStateActual(ctx, state); err != nil {
		return
	}
	// 设置期望的状态
	if err = r.setStateDesired(ctx, state); err != nil {
		return
	}
	return state, nil
}

type backupStateContainer struct {
	pod *corev1.Pod
}

func (r *EtcdBackupReconciler) setStateActual(ctx context.Context, state *backupState) (err error) {
	var actual backupStateContainer
	key := client.ObjectKey{
		Namespace: state.backup.Namespace,
		Name:      state.backup.Name,
	}
	actual.pod = &corev1.Pod{}
	if err := r.Get(ctx, key, actual.pod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return errors.New(fmt.Sprintf("failed to get pod :%s", err.Error()))
		}
		actual.pod = nil
	}
	state.actual = &actual
	return
}

func (r *EtcdBackupReconciler) setStateDesired(_ context.Context, state *backupState) (err error) {
	var desired backupStateContainer
	pod := podForBackup(state.backup, r.BackupImage)
	// 配置 controllerRef
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return ctrl.SetControllerReference(state.backup, pod, r.Scheme)
	}); err != nil {
		return errors.New(fmt.Sprintf("failed to set controller reference: %s", err.Error()))
	}

	desired.pod = pod
	// 获取期望的pod
	state.desired = &desired
	return
}

// podForBackup 期望的pod
func podForBackup(backup *v1alpha1.EtcdBackup, imgae string) *corev1.Pod {
	labels := map[string]string{
		"app": backup.Name,
	}
	var (
		Endpoint  string
		EnvFrom   = make([]corev1.EnvFromSource, 0, 0)
		backupURL string
	)
	switch backup.Spec.StorageType {
	case v1alpha1.BackupStrorageTypeOSS:
		Endpoint = backup.Spec.OSS.Endpint
		backupURL = fmt.Sprintf("%s://%s", backup.Spec.StorageType, backup.Spec.OSS.Path)
		EnvFrom = append(EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: backup.Spec.OSS.Secret,
				},
			},
		})
	case v1alpha1.BackupStrorageTypeS3:
		Endpoint = backup.Spec.S3.Endpoint
		backupURL = fmt.Sprintf("%s://%s", backup.Spec.StorageType, backup.Spec.S3.Path)
		EnvFrom = append(EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: backup.Spec.S3.Secret,
				},
			},
		})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "etcd-backup",
					Image: imgae,
					// 环境变量
					Args: []string{
						"--etcd-url",
						backup.Spec.EtcdUrl,
						"--backup-url",
						backupURL,
					},
					Env: []corev1.EnvVar{
						{
							Name:  "ENDPOINT",
							Value: Endpoint,
						},
					},
					EnvFrom: EnvFrom,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=etcd.io.etcd.io,resources=etcdbackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=,resources=pods/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EtcdBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *EtcdBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx)
	logger = logger.WithValues("etcdBackup", req.NamespacedName)
	state, err := r.getState(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	var action Action
	var (
		PhaseStatus = state.actual.pod.Status.Phase
	)
	switch {
	case state.backup == nil, !state.backup.DeletionTimestamp.IsZero(),
		// 备份失败
		state.backup.Status.Phase == v1alpha1.EtcdBackupPhaseFailed,
		state.backup.Status.Phase == v1alpha1.EtcdBackupPhaseCompleted:
		return
	case state.backup.Status.Phase == "": // 开始备份
		logger.Info("start Backup")
		// 当前的 backup 深拷贝
		newBackup := state.backup.DeepCopy()
		newBackup.Status.Phase = v1alpha1.EtcdBackupPhasePending
		action = &PatchStateus{
			Client:   r.Client,
			original: state.backup,
			new:      newBackup,
		}
	case state.actual.pod == nil:
		action = &CreateObject{
			Client: r.Client,
			obj:    state.desired.pod,
		}
		// 备份失败
	case PhaseStatus == corev1.PodFailed:
		newBackup := state.backup.DeepCopy()
		newBackup.Status.Phase = v1alpha1.EtcdBackupPhaseFailed
		action = &PatchStateus{
			Client:   r.Client,
			original: state.backup,
			new:      newBackup,
		}
		// 备份成功
	case PhaseStatus == corev1.PodSucceeded:
		newBackup := state.backup.DeepCopy()
		newBackup.Status.Phase = v1alpha1.EtcdBackupPhaseCompleted
		action = &PatchStateus{
			Client:   r.Client,
			original: state.backup,
			new:      newBackup,
		}
	}

	if action != nil {
		if err = action.Execute(ctx); err != nil {
			return
		}
	}

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdiov1alpha1.EtcdBackup{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
