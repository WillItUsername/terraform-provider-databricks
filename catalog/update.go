package catalog

import (
	"context"
	"path"

	"github.com/databricks/terraform-provider-databricks/common"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func updateFunctionFactory(pathPrefix string, updatable []string) func(context.Context, *schema.ResourceData, *common.DatabricksClient) error {
	return func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
		patch := map[string]any{}
		for _, field := range updatable {

			// these fields cannot be set during creation
			if d.IsNewResource() && !contains([]string{
				"owner",
				"delta_sharing_scope",
				"delta_sharing_recipient_token_lifetime_in_seconds",
				"delta_sharing_organization_name",
			}, field) {
				continue
			}

			var old, new any
			if field == "columns" {
				old, new = d.GetChange("column")
				if !d.HasChange("column") {
					continue
				}
			} else {
				old, new = d.GetChange(field)
				if !d.HasChange(field) {
					continue
				}
			}

			// need to reset the delta sharing token lifetime
			if field == "delta_sharing_scope" && old != new && new == "INTERNAL_AND_EXTERNAL" &&
				!d.HasChange("delta_sharing_recipient_token_lifetime_in_seconds") {
				patch["delta_sharing_recipient_token_lifetime_in_seconds"] =
					d.Get("delta_sharing_recipient_token_lifetime_in_seconds")
			}

			if contains([]string{
				"aws_iam_role",
				"azure_service_principal",
				"azure_managed_identity",
			}, field) {
				patch[field] = d.Get(field).([]any)[0]
				continue
			}

			patch[field] = new

		}
		if len(patch) == 0 {
			return nil
		}
		return c.Patch(context.WithValue(ctx, common.Api, common.API_2_1), path.Join(pathPrefix, d.Id()), patch)
	}
}