//go:build unit

package apicast

import (
	"context"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/3scale/apicast-operator/apis/apps/v1alpha1"
)

func TestPodLabelSelector(t *testing.T) {
	// This test must pass to ensure the upgrade procedure in picast_controller_deployment_upgrade.go
	// works as expected

	apicastConfigSecretName := "my-secret"
	namespace := "my-ns"

	embeddedConfigSecret := GetTestSecret(namespace, apicastConfigSecretName,
		map[string]string{"config.json": "{}"},
	)

	apicastCR := &appsv1alpha1.APIcast{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance1",
			Namespace: namespace,
		},
		Spec: appsv1alpha1.APIcastSpec{
			EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
				Name: apicastConfigSecretName,
			},
		},
	}

	objs := []runtime.Object{embeddedConfigSecret}
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	optsProvider := NewApicastOptionsProvider(apicastCR, cl)
	opts, err := optsProvider.GetApicastOptions(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	expectedPodLabelSelectors := map[string]string{"deployment": "apicast-instance1"}
	if !reflect.DeepEqual(expectedPodLabelSelectors, opts.PodLabelSelector) {
		t.Fatalf("PodLabelSelector not expected: %s",
			cmp.Diff(expectedPodLabelSelectors, opts.PodLabelSelector))
	}
}

func TestOpentelemetryOptions(t *testing.T) {
	namespace := "my-ns"
	apicastConfigSecretName := "my-secret"
	embeddedConfigSecret := GetTestSecret(namespace, apicastConfigSecretName,
		map[string]string{"config.json": "{}"},
	)

	t.Run("Secret ref not set", func(subT *testing.T) {
		apicastCR := &appsv1alpha1.APIcast{
			ObjectMeta: metav1.ObjectMeta{
				Name: "instance1", Namespace: namespace,
			},
			Spec: appsv1alpha1.APIcastSpec{
				EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
					Name: apicastConfigSecretName,
				},
				OpenTelemetry: &appsv1alpha1.OpenTelemetrySpec{
					Enabled: &[]bool{true}[0],
				},
			},
		}

		objs := []runtime.Object{embeddedConfigSecret}
		cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
		optsProvider := NewApicastOptionsProvider(apicastCR, cl)
		_, err := optsProvider.GetApicastOptions(context.TODO())
		if err == nil {
			subT.Fatal("get options should fail")
		}

		if !strings.Contains(err.Error(), "spec.openTelemetry.tracingConfigSecretRef: Invalid value") {
			subT.Fatalf("error unexpected: %s", err)
		}
	})

	t.Run("Secret key provided", func(subT *testing.T) {
		apicastCR := &appsv1alpha1.APIcast{
			ObjectMeta: metav1.ObjectMeta{
				Name: "instance1", Namespace: namespace,
			},
			Spec: appsv1alpha1.APIcastSpec{
				EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
					Name: apicastConfigSecretName,
				},
				OpenTelemetry: &appsv1alpha1.OpenTelemetrySpec{
					Enabled: &[]bool{true}[0],
					TracingConfigSecretRef: &v1.LocalObjectReference{
						Name: "secretName",
					},
					TracingConfigSecretKey: &[]string{"file1"}[0],
				},
			},
		}

		objs := []runtime.Object{embeddedConfigSecret}
		cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
		optsProvider := NewApicastOptionsProvider(apicastCR, cl)
		opts, err := optsProvider.GetApicastOptions(context.TODO())
		if err != nil {
			subT.Fatalf("get options should not fail: %s", err)
		}

		if opts == nil {
			subT.Fatal("options should not be nil")
		}

		expectedOtelOptions := OpentelemetryConfig{
			Enabled:    true,
			SecretName: "secretName",
			ConfigFile: path.Join(OpentelemetryConfigMountBasePath, "file1"),
		}

		if !reflect.DeepEqual(expectedOtelOptions, opts.Opentelemetry) {
			subT.Fatalf("opentelemetry object not expected: %s",
				cmp.Diff(expectedOtelOptions, opts.Opentelemetry))
		}
	})

	t.Run("Secret key not provided", func(subT *testing.T) {
		tracingConfigSecret := GetTestSecret(namespace, "otelSecret",
			map[string]string{
				"c.json": "{}",
				"b.json": "{}",
				"a.json": "{}",
			},
		)
		apicastCR := &appsv1alpha1.APIcast{
			ObjectMeta: metav1.ObjectMeta{
				Name: "instance1", Namespace: namespace,
			},
			Spec: appsv1alpha1.APIcastSpec{
				EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
					Name: apicastConfigSecretName,
				},
				OpenTelemetry: &appsv1alpha1.OpenTelemetrySpec{
					Enabled: &[]bool{true}[0],
					TracingConfigSecretRef: &v1.LocalObjectReference{
						Name: "otelSecret",
					},
				},
			},
		}

		objs := []runtime.Object{embeddedConfigSecret, tracingConfigSecret}
		cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
		optsProvider := NewApicastOptionsProvider(apicastCR, cl)
		opts, err := optsProvider.GetApicastOptions(context.TODO())
		if err != nil {
			subT.Fatalf("get options should not fail: %s", err)
		}

		if opts == nil {
			subT.Fatal("options should not be nil")
		}

		expectedOtelOptions := OpentelemetryConfig{
			Enabled:    true,
			SecretName: "otelSecret",
			ConfigFile: path.Join(OpentelemetryConfigMountBasePath, "a.json"),
		}

		if !reflect.DeepEqual(expectedOtelOptions, opts.Opentelemetry) {
			subT.Fatalf("opentelemetry object not expected: %s",
				cmp.Diff(expectedOtelOptions, opts.Opentelemetry))
		}
	})
}

func TestInvalidCustomCABundleOption(t *testing.T) {
	namespace := "my-ns"
	apicastConfigSecretName := "my-secret"
	embeddedConfigSecret := GetTestSecret(namespace, apicastConfigSecretName,
		map[string]string{"config.json": "{}"},
	)

	invalid_cacertConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cacert",
			Namespace: namespace,
		},
		Data: map[string]string{
			"a.crt": "{}",
		},
	}
	cases := []struct {
		testName      string
		configMapRef  *v1.LocalObjectReference
		configMap     *v1.ConfigMap
		expectedError string
	}{
		{
			"ConfigMap ref not set",
			&v1.LocalObjectReference{},
			nil,
			"spec.customCABundleConfigMapRef.name: Required value: configmap name not provided",
		},
		{
			"ConfigMap ref provided but configmap does not exist",
			&v1.LocalObjectReference{Name: "cacert"},
			nil,
			"configmaps \"cacert\" not found",
		},
		{
			"ConfigMap key not provided",
			&v1.LocalObjectReference{Name: "cacert"},
			invalid_cacertConfigMap,
			"Required value: Required configmap key, ca-bundle.crt not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.testName, func(subT *testing.T) {
			apicastCR := &appsv1alpha1.APIcast{
				ObjectMeta: metav1.ObjectMeta{
					Name: "instance1", Namespace: namespace,
				},
				Spec: appsv1alpha1.APIcastSpec{
					EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
						Name: apicastConfigSecretName,
					},
					CustomCABundleConfigMapRef: tc.configMapRef,
				},
			}

			objs := []runtime.Object{embeddedConfigSecret}
			if tc.configMap != nil {
				objs = append(objs, tc.configMap)
			}
			cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			optsProvider := NewApicastOptionsProvider(apicastCR, cl)
			_, err := optsProvider.GetApicastOptions(context.TODO())
			if err == nil {
				subT.Fatal("get options should fail")
			}

			if !strings.Contains(err.Error(), tc.expectedError) {
				subT.Fatalf("error unexpected: %s", err)
			}
		})
	}
}

func TestCustomCABundleOption(t *testing.T) {
	namespace := "my-ns"
	apicastConfigSecretName := "my-secret"
	embeddedConfigSecret := GetTestSecret(namespace, apicastConfigSecretName,
		map[string]string{"config.json": "{}"},
	)

	cacertConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cacert",
			Namespace: namespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": "{}",
		},
	}

	apicastCR := &appsv1alpha1.APIcast{
		ObjectMeta: metav1.ObjectMeta{
			Name: "instance1", Namespace: namespace,
		},
		Spec: appsv1alpha1.APIcastSpec{
			EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
				Name: apicastConfigSecretName,
			},
			CustomCABundleConfigMapRef: &v1.LocalObjectReference{
				Name: "cacert",
			},
		},
	}

	objs := []runtime.Object{embeddedConfigSecret, cacertConfigMap}
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	optsProvider := NewApicastOptionsProvider(apicastCR, cl)
	opts, err := optsProvider.GetApicastOptions(context.TODO())
	if err != nil {
		t.Fatalf("get options should not fail: %s", err)
	}

	if opts == nil {
		t.Fatal("options should not be nil")
	}

	if !reflect.DeepEqual(opts.CustomCABundleConfigMap, cacertConfigMap) {
		t.Fatalf("cacert configmap mismatch: %s",
			cmp.Diff(cacertConfigMap, opts.CustomCABundleConfigMap))
	}
}

func TestExposedHostOption(t *testing.T) {
	namespace := "my-ns"
	apicastConfigSecretName := "my-secret"
	embeddedConfigSecret := GetTestSecret(namespace, apicastConfigSecretName,
		map[string]string{"config.json": "{}"},
	)
	cases := []struct {
		testName    string
		exposedHost *appsv1alpha1.APIcastExposedHost
		expected    ExposedHost
	}{
		{
			"empty host",
			&appsv1alpha1.APIcastExposedHost{},
			ExposedHost{},
		},
		{
			"with host",
			&appsv1alpha1.APIcastExposedHost{Host: "example"},
			ExposedHost{Host: "example"},
		},
		{
			"with ingressClassName",
			&appsv1alpha1.APIcastExposedHost{
				Host:             "example",
				IngressClassName: ptr.To("default"),
			},
			ExposedHost{
				Host:             "example",
				IngressClassName: ptr.To("default"),
			},
		},
		{
			"with TLS",
			&appsv1alpha1.APIcastExposedHost{
				Host: "example",
				TLS: []networkingv1.IngressTLS{
					{Hosts: []string{"example"}},
				},
			},
			ExposedHost{
				Host: "example",
				TLS: []networkingv1.IngressTLS{
					{Hosts: []string{"example"}},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.testName, func(subT *testing.T) {
			apicastCR := &appsv1alpha1.APIcast{
				ObjectMeta: metav1.ObjectMeta{
					Name: "instance1", Namespace: namespace,
				},
				Spec: appsv1alpha1.APIcastSpec{
					EmbeddedConfigurationSecretRef: &v1.LocalObjectReference{
						Name: apicastConfigSecretName,
					},
					ExposedHost: tc.exposedHost,
				},
			}

			objs := []runtime.Object{embeddedConfigSecret}
			cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			optsProvider := NewApicastOptionsProvider(apicastCR, cl)
			opts, err := optsProvider.GetApicastOptions(context.TODO())
			if err != nil {
				t.Fatalf("get options should not fail: %s", err)
			}

			if opts == nil {
				t.Fatal("options should not be nil")
			}

			if !reflect.DeepEqual(opts.ExposedHost, tc.expected) {
				t.Fatalf("cacert secret mismatch: %s",
					cmp.Diff(tc.expected, opts.ExposedHost))
			}
		})
	}
}
