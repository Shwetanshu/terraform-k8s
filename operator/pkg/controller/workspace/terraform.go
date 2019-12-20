package workspace

import (
	"bytes"
	"context"
	"text/template"

	"github.com/hashicorp/terraform-k8s/operator/pkg/apis/app/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	TerraformConfigMap = "terraform"
	TerraformOperator  = "terraform-k8s"
)

// CreateTerraformTemplate creates a template for the Terraform configuration
func CreateTerraformTemplate(workspace *v1alpha1.Workspace) ([]byte, error) {
	tfTemplate, err := template.New("main.tf").Parse(`terraform {
		backend "remote" {
			organization = "{{.Spec.Organization}}"
	
			workspaces {
				name = "{{.ObjectMeta.Namespace}}-{{.ObjectMeta.Name}}"
			}
		}
	}
	{{- range .Spec.Variables}}
	{{- if not .EnvironmentVariable }}
	variable "{{.Key}}" {}
	{{- end}}
	{{- end}}
	{{- range .Spec.Outputs}}
	output "{{.Key}}" {
		value = module.operator.{{.Attribute}}
	}
	{{- end}}
	module "operator" {
		source = "{{.Spec.Module.Source}}"
		version = "{{.Spec.Module.Version}}"
		{{- range .Spec.Variables}}
		{{- if not .EnvironmentVariable }}
		{{.Key}} = var.{{.Key}}
		{{- end}}
		{{- end}}
	}`)
	if err != nil {
		return nil, err
	}
	var tpl bytes.Buffer
	if err := tfTemplate.Execute(&tpl, workspace); err != nil {
		return nil, err
	}
	return tpl.Bytes(), nil
}

func configMapForTerraform(w *v1alpha1.Workspace, template []byte) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Namespace,
			Labels: map[string]string{
				"owned-by": TerraformOperator,
			},
		},
		Data: map[string]string{
			TerraformConfigMap: string(template),
		},
	}
}

func (r *ReconcileWorkspace) UpsertConfigMap(w *v1alpha1.Workspace, template []byte) (bool, error) {
	updated := false
	found := &v1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: w.Name, Namespace: w.Namespace}, found)
	configMap := configMapForTerraform(w, template)
	if err != nil && errors.IsNotFound(err) {
		r.reqLogger.Info("Writing terraform to new ConfigMap")
		if err := r.client.Create(context.TODO(), configMap); err != nil {
			r.reqLogger.Error(err, "Failed to create new ConfigMap")
			return updated, err
		}
	} else if err != nil {
		r.reqLogger.Error(err, "Failed to get ConfigMap")
		return updated, err
	}

	if found.Data[TerraformConfigMap] != configMap.Data[TerraformConfigMap] {
		found.Data = configMap.Data
		if err := r.client.Update(context.TODO(), found); err != nil {
			r.reqLogger.Error(err, "Failed to update ConfigMap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return updated, err
		}
		return true, nil
	}
	return updated, nil
}
