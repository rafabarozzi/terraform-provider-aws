// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package docdb

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/docdb"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	tfslices "github.com/hashicorp/terraform-provider-aws/internal/slices"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKResource("aws_docdb_cluster_instance", name="Cluster Instance")
// @Tags(identifierAttribute="arn")
func ResourceClusterInstance() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceClusterInstanceCreate,
		ReadWithoutTimeout:   resourceClusterInstanceRead,
		UpdateWithoutTimeout: resourceClusterInstanceUpdate,
		DeleteWithoutTimeout: resourceClusterInstanceDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(90 * time.Minute),
			Update: schema.DefaultTimeout(90 * time.Minute),
			Delete: schema.DefaultTimeout(90 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"apply_immediately": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			names.AttrARN: {
				Type:     schema.TypeString,
				Computed: true,
			},
			names.AttrAutoMinorVersionUpgrade: {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			names.AttrAvailabilityZone: {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"ca_cert_identifier": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			names.AttrClusterIdentifier: {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"copy_tags_to_snapshot": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"db_subnet_group_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"dbi_resource_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"enable_performance_insights": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			names.AttrEndpoint: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"engine": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				Default:      engineDocDB,
				ValidateFunc: validation.StringInSlice(engine_Values(), false),
			},
			names.AttrEngineVersion: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"identifier": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"identifier_prefix"},
				ValidateFunc:  validIdentifier,
			},
			"identifier_prefix": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validIdentifierPrefix,
			},
			"instance_class": {
				Type:     schema.TypeString,
				Required: true,
			},
			names.AttrKMSKeyID: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"performance_insights_kms_key_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			names.AttrPort: {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"preferred_backup_window": {
				Type:     schema.TypeString,
				Computed: true,
			},
			names.AttrPreferredMaintenanceWindow: {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				StateFunc: func(v interface{}) string {
					if v != nil {
						value := v.(string)
						return strings.ToLower(value)
					}
					return ""
				},
				ValidateFunc: verify.ValidOnceAWeekWindowFormat,
			},
			"promotion_tier": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      0,
				ValidateFunc: validation.IntBetween(0, 15),
			},
			"publicly_accessible": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"storage_encrypted": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			names.AttrTags:    tftags.TagsSchema(),
			names.AttrTagsAll: tftags.TagsSchemaComputed(),
			"writer": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},

		CustomizeDiff: verify.SetTagsDiff,
	}
}

func resourceClusterInstanceCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).DocDBConn(ctx)

	identifier := create.NewNameGenerator(
		create.WithConfiguredName(d.Get("identifier").(string)),
		create.WithConfiguredPrefix(d.Get("identifier_prefix").(string)),
		create.WithDefaultPrefix("tf-"),
	).Generate()
	input := &docdb.CreateDBInstanceInput{
		AutoMinorVersionUpgrade: aws.Bool(d.Get(names.AttrAutoMinorVersionUpgrade).(bool)),
		DBClusterIdentifier:     aws.String(d.Get(names.AttrClusterIdentifier).(string)),
		DBInstanceClass:         aws.String(d.Get("instance_class").(string)),
		DBInstanceIdentifier:    aws.String(identifier),
		Engine:                  aws.String(d.Get("engine").(string)),
		PromotionTier:           aws.Int64(int64(d.Get("promotion_tier").(int))),
		Tags:                    getTagsIn(ctx),
	}

	if v, ok := d.GetOk(names.AttrAvailabilityZone); ok {
		input.AvailabilityZone = aws.String(v.(string))
	}

	if v, ok := d.GetOk("copy_tags_to_snapshot"); ok {
		input.CopyTagsToSnapshot = aws.Bool(v.(bool))
	}

	if v, ok := d.GetOk("enable_performance_insights"); ok {
		input.EnablePerformanceInsights = aws.Bool(v.(bool))
	}

	if v, ok := d.GetOk("performance_insights_kms_key_id"); ok {
		input.PerformanceInsightsKMSKeyId = aws.String(v.(string))
	}

	if v, ok := d.GetOk(names.AttrPreferredMaintenanceWindow); ok {
		input.PreferredMaintenanceWindow = aws.String(v.(string))
	}

	_, err := tfresource.RetryWhenAWSErrMessageContains(ctx, propagationTimeout, func() (interface{}, error) {
		return conn.CreateDBInstanceWithContext(ctx, input)
	}, errCodeInvalidParameterValue, "IAM role ARN value is invalid or does not include the required permissions")

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating DocumentDB Cluster Instance (%s): %s", identifier, err)
	}

	d.SetId(identifier)

	if _, err := waitDBInstanceAvailable(ctx, conn, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for DocumentDB Cluster Instance (%s) create: %s", d.Id(), err)
	}

	return append(diags, resourceClusterInstanceRead(ctx, d, meta)...)
}

func resourceClusterInstanceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).DocDBConn(ctx)

	db, err := FindDBInstanceByID(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] DocumentDB Cluster Instance (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading DocumentDB Cluster Instance (%s): %s", d.Id(), err)
	}

	clusterID := aws.StringValue(db.DBClusterIdentifier)
	dbc, err := FindDBClusterByID(ctx, conn, clusterID)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading DocumentDB Cluster (%s): %s", clusterID, err)
	}

	d.Set(names.AttrARN, db.DBInstanceArn)
	d.Set(names.AttrAutoMinorVersionUpgrade, db.AutoMinorVersionUpgrade)
	d.Set(names.AttrAvailabilityZone, db.AvailabilityZone)
	d.Set("ca_cert_identifier", db.CACertificateIdentifier)
	d.Set(names.AttrClusterIdentifier, db.DBClusterIdentifier)
	d.Set("copy_tags_to_snapshot", db.CopyTagsToSnapshot)
	if db.DBSubnetGroup != nil {
		d.Set("db_subnet_group_name", db.DBSubnetGroup.DBSubnetGroupName)
	}
	d.Set("dbi_resource_id", db.DbiResourceId)
	// The AWS API does not expose 'EnablePerformanceInsights' the line below should be uncommented
	// as soon as it is available in the DescribeDBClusters output.
	//d.Set("enable_performance_insights", db.EnablePerformanceInsights)
	if db.Endpoint != nil {
		d.Set(names.AttrEndpoint, db.Endpoint.Address)
		d.Set(names.AttrPort, db.Endpoint.Port)
	}
	d.Set("engine", db.Engine)
	d.Set(names.AttrEngineVersion, db.EngineVersion)
	d.Set("identifier", db.DBInstanceIdentifier)
	d.Set("identifier_prefix", create.NamePrefixFromName(aws.StringValue(db.DBInstanceIdentifier)))
	d.Set("instance_class", db.DBInstanceClass)
	d.Set(names.AttrKMSKeyID, db.KmsKeyId)
	// The AWS API does not expose 'PerformanceInsightsKMSKeyId'  the line below should be uncommented
	// as soon as it is available in the DescribeDBClusters output.
	//d.Set("performance_insights_kms_key_id", db.PerformanceInsightsKMSKeyId)
	d.Set("preferred_backup_window", db.PreferredBackupWindow)
	d.Set(names.AttrPreferredMaintenanceWindow, db.PreferredMaintenanceWindow)
	d.Set("promotion_tier", db.PromotionTier)
	d.Set("publicly_accessible", db.PubliclyAccessible)
	d.Set("storage_encrypted", db.StorageEncrypted)
	if v := tfslices.Filter(dbc.DBClusterMembers, func(v *docdb.DBClusterMember) bool {
		return aws.StringValue(v.DBInstanceIdentifier) == d.Id()
	}); len(v) == 1 {
		d.Set("writer", v[0].IsClusterWriter)
	}

	return diags
}

func resourceClusterInstanceUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).DocDBConn(ctx)

	if d.HasChangesExcept(names.AttrTags, names.AttrTagsAll) {
		input := &docdb.ModifyDBInstanceInput{
			ApplyImmediately:     aws.Bool(d.Get("apply_immediately").(bool)),
			DBInstanceIdentifier: aws.String(d.Id()),
		}

		if d.HasChange(names.AttrAutoMinorVersionUpgrade) {
			input.AutoMinorVersionUpgrade = aws.Bool(d.Get(names.AttrAutoMinorVersionUpgrade).(bool))
		}

		if d.HasChange("ca_cert_identifier") {
			input.CACertificateIdentifier = aws.String(d.Get("ca_cert_identifier").(string))
		}

		if d.HasChange("copy_tags_to_snapshot") {
			input.CopyTagsToSnapshot = aws.Bool(d.Get("copy_tags_to_snapshot").(bool))
		}

		if d.HasChange("enable_performance_insights") {
			input.EnablePerformanceInsights = aws.Bool(d.Get("enable_performance_insights").(bool))
		}

		if d.HasChange("instance_class") {
			input.DBInstanceClass = aws.String(d.Get("instance_class").(string))
		}

		if d.HasChange("performance_insights_kms_key_id") {
			input.PerformanceInsightsKMSKeyId = aws.String(d.Get("performance_insights_kms_key_id").(string))
		}

		if d.HasChange(names.AttrPreferredMaintenanceWindow) {
			input.PreferredMaintenanceWindow = aws.String(d.Get(names.AttrPreferredMaintenanceWindow).(string))
		}

		if d.HasChange("promotion_tier") {
			input.PromotionTier = aws.Int64(int64(d.Get("promotion_tier").(int)))
		}

		_, err := tfresource.RetryWhenAWSErrMessageContains(ctx, propagationTimeout, func() (interface{}, error) {
			return conn.ModifyDBInstanceWithContext(ctx, input)
		}, errCodeInvalidParameterValue, "IAM role ARN value is invalid or does not include the required permissions")

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "modifying DocumentDB Cluster Instance (%s): %s", d.Id(), err)
		}

		if _, err := waitDBInstanceAvailable(ctx, conn, d.Id(), d.Timeout(schema.TimeoutUpdate)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for DocumentDB Cluster Instance (%s) update: %s", d.Id(), err)
		}
	}

	return append(diags, resourceClusterInstanceRead(ctx, d, meta)...)
}

func resourceClusterInstanceDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).DocDBConn(ctx)

	log.Printf("[DEBUG] Deleting DocumentDB Cluster Instance: %s", d.Id())
	_, err := conn.DeleteDBInstanceWithContext(ctx, &docdb.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(d.Id()),
	})

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting DocumentDB Cluster Instance (%s): %s", d.Id(), err)
	}

	if _, err := waitDBInstanceDeleted(ctx, conn, d.Id(), d.Timeout(schema.TimeoutDelete)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for DocumentDB Cluster Instance (%s) delete: %s", d.Id(), err)
	}

	return diags
}

func FindDBInstanceByID(ctx context.Context, conn *docdb.DocDB, id string) (*docdb.DBInstance, error) {
	input := &docdb.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(id),
	}
	output, err := findDBInstance(ctx, conn, input)

	if err != nil {
		return nil, err
	}

	// Eventual consistency check.
	if aws.StringValue(output.DBInstanceIdentifier) != id {
		return nil, &retry.NotFoundError{
			LastRequest: input,
		}
	}

	return output, nil
}

func findDBInstance(ctx context.Context, conn *docdb.DocDB, input *docdb.DescribeDBInstancesInput) (*docdb.DBInstance, error) {
	output, err := findDBInstances(ctx, conn, input)

	if err != nil {
		return nil, err
	}

	return tfresource.AssertSinglePtrResult(output)
}

func findDBInstances(ctx context.Context, conn *docdb.DocDB, input *docdb.DescribeDBInstancesInput) ([]*docdb.DBInstance, error) {
	var output []*docdb.DBInstance

	err := conn.DescribeDBInstancesPagesWithContext(ctx, input, func(page *docdb.DescribeDBInstancesOutput, lastPage bool) bool {
		if page == nil {
			return !lastPage
		}

		for _, v := range page.DBInstances {
			if v != nil {
				output = append(output, v)
			}
		}

		return !lastPage
	})

	if tfawserr.ErrCodeEquals(err, docdb.ErrCodeDBInstanceNotFoundFault) {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: input,
		}
	}

	if err != nil {
		return nil, err
	}

	return output, nil
}

func statusDBInstance(ctx context.Context, conn *docdb.DocDB, id string) retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := FindDBInstanceByID(ctx, conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.DBInstanceStatus), nil
	}
}

func waitDBInstanceAvailable(ctx context.Context, conn *docdb.DocDB, id string, timeout time.Duration) (*docdb.DBInstance, error) { //nolint:unparam
	stateConf := &retry.StateChangeConf{
		Pending: []string{
			"backing-up",
			"configuring-enhanced-monitoring",
			"configuring-iam-database-auth",
			"configuring-log-exports",
			"creating",
			"maintenance",
			"modifying",
			"rebooting",
			"renaming",
			"resetting-master-credentials",
			"starting",
			"storage-optimization",
			"upgrading",
		},
		Target:     []string{"available"},
		Refresh:    statusDBInstance(ctx, conn, id),
		Timeout:    timeout,
		MinTimeout: 10 * time.Second,
		Delay:      30 * time.Second,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*docdb.DBInstance); ok {
		return output, err
	}

	return nil, err
}

func waitDBInstanceDeleted(ctx context.Context, conn *docdb.DocDB, id string, timeout time.Duration) (*docdb.DBInstance, error) {
	stateConf := &retry.StateChangeConf{
		Pending: []string{
			"configuring-log-exports",
			"modifying",
			"deleting",
		},
		Target:     []string{},
		Refresh:    statusDBInstance(ctx, conn, id),
		Timeout:    timeout,
		MinTimeout: 10 * time.Second,
		Delay:      30 * time.Second,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*docdb.DBInstance); ok {
		return output, err
	}

	return nil, err
}
