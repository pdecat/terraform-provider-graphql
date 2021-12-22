package graphql

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"github.com/zclconf/go-cty/cty/gocty"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"url": {
				Required:    true,
				Type:        schema.TypeString,
				ForceNew:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_GRAPHQL_URL", nil),
			},
			"headers": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
				ForceNew: true,
			},
			"login_query": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"login_query_variables": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
				ForceNew: true,
			},
			"authenticated_headers": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
				ForceNew: true,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"graphql_mutation": resourceGraphqlMutation(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"graphql_query": dataSourceGraphql(),
		},
		ConfigureContextFunc: graphqlConfigure,
	}
}

func graphqlConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	config := &graphqlProviderConfig{
		GQLServerUrl:   d.Get("url").(string),
		RequestHeaders: d.Get("headers").(map[string]interface{}),
	}

	if d.Get("login_query") != "" {
		queryResponse, resBytes, err := queryExecute(ctx, d, config, "login_query", "login_query_variables")
		if err != nil {
			return nil, diag.FromErr(fmt.Errorf("unable to execute read login_query: %w", err))
		}

		if queryErrors := queryResponse.ProcessErrors(); queryErrors.HasError() {
			return nil, *queryErrors
		}

		var queryResponseCty cty.Value
		if queryResponseCty, err = gocty.ToCtyValue(string(resBytes), cty.String); err != nil {
			return nil, diag.FromErr(err)
		}

		evalCtx := &hcl.EvalContext{
			Variables: map[string]cty.Value{
				"login_query_response": queryResponseCty,
			},
			Functions: map[string]function.Function{
				"jsondecode": stdlib.JSONDecodeFunc,
				//TODO: maybe add other useful functions
			},
		}

		config.RequestAuthenticatedHeaders = make(map[string]interface{})
		for k, v := range d.Get("authenticated_headers").(map[string]interface{}) {
			var expr hcl.Expression
			var hclDiags hcl.Diagnostics
			if expr, hclDiags = hclsyntax.ParseTemplate([]byte(v.(string)), k, hcl.InitialPos); len(hclDiags) > 0 {
				return nil, convertDiagnosticsFromHCLToTerraformSDK(hclDiags)
			}

			var interpolated cty.Value
			if interpolated, hclDiags = expr.Value(evalCtx); len(hclDiags) > 0 {
				return nil, convertDiagnosticsFromHCLToTerraformSDK(hclDiags)
			}
			config.RequestAuthenticatedHeaders[k] = interpolated.AsString()
		}
	}

	return config, diag.Diagnostics{}
}

type graphqlProviderConfig struct {
	GQLServerUrl   string
	RequestHeaders map[string]interface{}

	RequestAuthenticatedHeaders map[string]interface{}
}

func convertDiagnosticsFromHCLToTerraformSDK(hclDiags hcl.Diagnostics) diag.Diagnostics {
	diags := make(diag.Diagnostics, len(hclDiags))
	for i, hclDiag := range hclDiags {
		// HCL severity enum is: 0 (invalid), 1 (error), 2 (warning)
		// terraform SDK severity enum is: 0 (error), 1 (warning)
		diags[i].Severity = diag.Severity(hclDiag.Severity - 1)
		diags[i].Summary = hclDiag.Summary
		diags[i].Detail = hclDiag.Detail
		// diags[i].AttributePath = ?
	}
	return diags
}
