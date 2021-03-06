/*
Copyright 2019 The Knative Authors

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

package resources

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"knative.dev/pkg/apis/duck"
	fakedynamicclient "knative.dev/pkg/injection/clients/dynamicclient/fake"
	"knative.dev/serving/pkg/apis/serving"

	podscalable "knative.dev/serving/pkg/client/injection/ducks/autoscaling/v1alpha1/podscalable/fake"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	. "knative.dev/pkg/reconciler/testing"
)

func TestScaleResource(t *testing.T) {
	cases := []struct {
		name      string
		objectRef corev1.ObjectReference
		wantGVR   *schema.GroupVersionResource
		wantName  string
		wantErr   bool
	}{{
		name: "all good",
		objectRef: corev1.ObjectReference{
			Name:       "test",
			APIVersion: "apps/v1",
			Kind:       "deployment",
		},
		wantGVR: &schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		wantName: "test",
	}, {
		name: "broken apiversion",
		objectRef: corev1.ObjectReference{
			Name:       "test",
			APIVersion: "apps///v1",
			Kind:       "deployment",
		},
		wantErr: true,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gvr, name, err := ScaleResourceArguments(tc.objectRef)

			if !cmp.Equal(gvr, tc.wantGVR) {
				t.Errorf("ScaleResourceArguments() = %v, want: %v, diff: %s", gvr, tc.wantGVR, cmp.Diff(gvr, tc.wantGVR))
			}

			if name != tc.wantName {
				t.Errorf("ScaleResourceArguments() = %s, want %s", name, tc.wantName)
			}

			if err == nil && tc.wantErr {
				t.Error("ScaleResourceArguments() didn't return an error")
			}
			if err != nil && !tc.wantErr {
				t.Errorf("ScaleResourceArguments() = %v, want no error", err)
			}
		})
	}
}

func TestGetScaleResource(t *testing.T) {
	ctx, _ := SetupFakeContext(t)

	deployment := newDeployment(ctx, t, fakedynamicclient.Get(ctx), "testdeployment", 5)

	psInformerFactory := podscalable.Get(ctx)
	objectRef := corev1.ObjectReference{
		Name:       deployment.Name,
		Kind:       "deployment",
		APIVersion: "apps/v1",
	}
	scale, err := GetScaleResource(testNamespace, objectRef, func(gvr schema.GroupVersionResource) (cache.GenericLister, error) {
		_, l, err := psInformerFactory.Get(ctx, gvr)
		return l, err
	})
	if err != nil {
		t.Fatal("GetScaleResource() got error =", err)
	}
	if got, want := scale.Status.Replicas, int32(5); got != want {
		t.Errorf("GetScaleResource().Status.Replicas = %d, want: %d", got, want)
	}
	if got, want := scale.Spec.Selector.MatchLabels[serving.RevisionUID], "1982"; got != want {
		t.Errorf("GetScaleResource().Status.Selector = %q, want = %q", got, want)
	}
}

func newDeployment(ctx context.Context, t *testing.T, dynamicClient dynamic.Interface, name string, replicas int) *v1.Deployment {
	t.Helper()

	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"namespace": testNamespace,
				"name":      name,
				"uid":       "1982",
			},
			"spec": map[string]interface{}{
				"replicas": int64(replicas),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						serving.RevisionUID: "1982",
					},
				},
			},
			"status": map[string]interface{}{
				"replicas": int64(replicas),
			},
		},
	}

	u, err := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}).Namespace(testNamespace).Create(ctx, uns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("Create() =", err)
	}

	deployment := &v1.Deployment{}
	if err := duck.FromUnstructured(u, deployment); err != nil {
		t.Fatal("FromUnstructured() =", err)
	}
	return deployment
}
