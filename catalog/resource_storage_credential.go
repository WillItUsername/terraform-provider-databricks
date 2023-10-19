package catalog

import (
	"context"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/catalog"
	"github.com/databricks/terraform-provider-databricks/common"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type StorageCredentialInfo struct {
	Name        string                                       `json:"name" tf:"force_new"`
	Owner       string                                       `json:"owner,omitempty" tf:"computed"`
	Comment     string                                       `json:"comment,omitempty"`
	Aws         AwsIamRole                                   `json:"aws_iam_role,omitempty" tf:"group:access"`
	Azure       *catalog.AzureServicePrincipal               `json:"azure_service_principal,omitempty" tf:"group:access"`
	AzMI        *catalog.AzureManagedIdentity                `json:"azure_managed_identity,omitempty" tf:"group:access"`
	GcpSAKey    *GcpServiceAccountKey                        `json:"gcp_service_account_key,omitempty" tf:"group:access"`
	DBGcpSA     *catalog.DatabricksGcpServiceAccountResponse `json:"databricks_gcp_service_account,omitempty" tf:"computed"`
	MetastoreID string                                       `json:"metastore_id,omitempty" tf:"computed"`
	ReadOnly    bool                                         `json:"read_only,omitempty"`
}

func removeGcpSaField(originalSchema map[string]*schema.Schema) map[string]*schema.Schema {
	//common.DataToStructPointer(d, s, &create) will error out because of DatabricksGcpServiceAccount any
	tmpSchema := make(map[string]*schema.Schema)
	for k, v := range originalSchema {
		tmpSchema[k] = v
	}
	delete(tmpSchema, "databricks_gcp_service_account")
	return tmpSchema
}

var storageCredentialSchema = common.StructToSchema(StorageCredentialInfo{},
	func(m map[string]*schema.Schema) map[string]*schema.Schema {
		return adjustDataAccessSchema(m)
	})

func ResourceStorageCredential() *schema.Resource {
	return common.Resource{
		Schema: storageCredentialSchema,
		Create: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
			metastoreId := d.Get("metastore_id").(string)
			tmpSchema := removeGcpSaField(storageCredentialSchema)

			var create catalog.CreateStorageCredential
			var update catalog.UpdateStorageCredential
			common.DataToStructPointer(d, tmpSchema, &create)
			common.DataToStructPointer(d, tmpSchema, &update)

			//manually add empty struct back for databricks_gcp_service_account
			if _, ok := d.GetOk("databricks_gcp_service_account"); ok {
				create.DatabricksGcpServiceAccount = struct{}{}
			}

			return c.AccountOrWorkspaceRequest(func(acc *databricks.AccountClient) error {
				storageCredential, err := acc.StorageCredentials.Create(ctx,
					catalog.AccountsCreateStorageCredential{
						MetastoreId:    metastoreId,
						CredentialInfo: &create,
					})
				if err != nil {
					return err
				}
				d.SetId(storageCredential.CredentialInfo.Name)

				// Don't update owner if it is not provided
				if d.Get("owner") == "" {
					return nil
				}
				_, err = acc.StorageCredentials.Update(ctx, catalog.AccountsUpdateStorageCredential{
					CredentialInfo:        &update,
					MetastoreId:           metastoreId,
					StorageCredentialName: storageCredential.CredentialInfo.Id,
				})
				if err != nil {
					return err
				}
				return nil
			}, func(w *databricks.WorkspaceClient) error {
				storageCredential, err := w.StorageCredentials.Create(ctx, create)
				if err != nil {
					return err
				}
				d.SetId(storageCredential.Name)

				// Don't update owner if it is not provided
				if d.Get("owner") == "" {
					return nil
				}

				_, err = w.StorageCredentials.Update(ctx, update)
				if err != nil {
					return err
				}
				return nil
			})
		},
		Read: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {

			return c.AccountOrWorkspaceRequest(func(acc *databricks.AccountClient) error {
				storageCredential, err := acc.StorageCredentials.Get(ctx, catalog.GetAccountStorageCredentialRequest{
					MetastoreId:           d.Get("metastore_id").(string),
					StorageCredentialName: d.Id(),
				})
				if err != nil {
					return err
				}
				return common.StructToData(storageCredential.CredentialInfo, storageCredentialSchema, d)
			}, func(w *databricks.WorkspaceClient) error {
				storageCredential, err := w.StorageCredentials.GetByName(ctx, d.Id())
				if err != nil {
					return err
				}
				return common.StructToData(storageCredential, storageCredentialSchema, d)
			})
		},
		Update: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
			var update catalog.UpdateStorageCredential
			common.DataToStructPointer(d, storageCredentialSchema, &update)

			return c.AccountOrWorkspaceRequest(func(acc *databricks.AccountClient) error {
				_, err := acc.StorageCredentials.Update(ctx, catalog.AccountsUpdateStorageCredential{
					CredentialInfo:        &update,
					MetastoreId:           d.Get("metastore_id").(string),
					StorageCredentialName: d.Id(),
				})
				if err != nil {
					return err
				}
				return nil
			}, func(w *databricks.WorkspaceClient) error {
				_, err := w.StorageCredentials.Update(ctx, update)
				if err != nil {
					return err
				}
				return nil
			})
		},
		Delete: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
			force := d.Get("force_destroy").(bool)
			return c.AccountOrWorkspaceRequest(func(acc *databricks.AccountClient) error {
				return acc.StorageCredentials.Delete(ctx, catalog.DeleteAccountStorageCredentialRequest{
					Force:                 force,
					StorageCredentialName: d.Id(),
					MetastoreId:           d.Get("metastore_id").(string),
				})
			}, func(w *databricks.WorkspaceClient) error {
				return w.StorageCredentials.Delete(ctx, catalog.DeleteStorageCredentialRequest{
					Force: force,
					Name:  d.Id(),
				})
			})
		},
	}.ToResource()
}
