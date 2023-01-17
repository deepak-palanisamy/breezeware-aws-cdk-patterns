package ecs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	ec2 "github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	ecs "github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	iam "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type LoadBalancedEc2ServiceProps struct {
	Cluster              ClusterProps
	TaskDefinition       TaskDefinition
	DesiredTaskCount     float64
	ServiceHealthPercent ServiceHealthPercent
}

type ClusterProps struct {
	ClusterName string
	Vpc         VpcProps
}

type VpcProps struct {
	Id             string
	IsDefault      bool
	SecurityGroups []ec2.ISecurityGroup
}

type TaskDefinition struct {
	FamilyName            string
	Cpu                   string
	MemoryInMiB           string
	EnvironmentFileBucket string
	TaskPolicy            iam.PolicyDocument
	Containers            []ecs.ContainerDefinition
}

type ServiceHealthPercent struct {
}

type loadBalancedEc2Service struct {
	constructs.Construct
	ec2Service ecs.Ec2Service
}

type LoadBalancedEc2Service interface {
	Service() ecs.Ec2Service
}

func (s *loadBalancedEc2Service) Service() ecs.Ec2Service {
	return s.ec2Service
}

func NewLoadBalancedEc2Service(scope constructs.Construct, id *string, props *LoadBalancedEc2ServiceProps) LoadBalancedEc2Service {
	this := constructs.NewConstruct(scope, id)

	taskPolicyDocument := props.TaskDefinition.TaskPolicy
	taskPolicyDocument.AddStatements(
		setupTaskContainerDefaultXrayPolciyStatement(),
	)

	ecrRepositories := []*string{}
	for _, container := range props.TaskDefinition.Containers {
		ecrRepositories = append(ecrRepositories, container.ContainerName())
	}

	taskDef := ecs.NewTaskDefinition(this, jsii.String("Ec2TaskDefinition"), &ecs.TaskDefinitionProps{
		Family:        jsii.String(props.TaskDefinition.FamilyName),
		Cpu:           jsii.String(props.TaskDefinition.Cpu),
		MemoryMiB:     jsii.String(props.TaskDefinition.MemoryInMiB),
		Compatibility: ecs.Compatibility_EC2,
		NetworkMode:   ecs.NetworkMode_BRIDGE,
		ExecutionRole: iam.NewRole(this, jsii.String("ExecutionRole"), &iam.RoleProps{
			AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
			// TODO: update policy for Execution Role. replace managed policy with bucket specific inline policy
			ManagedPolicies: &[]iam.IManagedPolicy{
				iam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AmazonS3ReadOnlyAccess")),
			},
			InlinePolicies: &map[string]iam.PolicyDocument{
				*jsii.String("DefaultPolicy"): iam.NewPolicyDocument(
					&iam.PolicyDocumentProps{
						AssignSids: jsii.Bool(true),
						Statements: &[]iam.PolicyStatement{
							createEnvironmentFileBucketReadOnlyAccessPolicyStatement(props.TaskDefinition.EnvironmentFileBucket),
							createEcrContainerRegistryReadonlyAccessPolicyStatement(ecrRepositories),
						},
					},
				),
			},
		}),
		TaskRole: iam.NewRole(this, jsii.String("TaskRole"), &iam.RoleProps{
			AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
			InlinePolicies: &map[string]iam.PolicyDocument{
				*jsii.String("DefaultPolicy"): taskPolicyDocument,
			},
		}),
	})

	ec2Service := ecs.NewEc2Service(this, jsii.String("Ec2Service"), &ecs.Ec2ServiceProps{
		Cluster: ecs.Cluster_FromClusterAttributes(this, jsii.String("Cluster"), &ecs.ClusterAttributes{
			ClusterName:    jsii.String(props.Cluster.ClusterName),
			Vpc:            lookupVpc(this, id, &props.Cluster.Vpc),
			SecurityGroups: &props.Cluster.Vpc.SecurityGroups,
		}),
		TaskDefinition: taskDef,
		DesiredCount:   &props.DesiredTaskCount,
		CircuitBreaker: &ecs.DeploymentCircuitBreaker{
			Rollback: jsii.Bool(true),
		},
	})

	return &loadBalancedEc2Service{this, ec2Service}
}

func lookupVpc(scope constructs.Construct, id *string, props *VpcProps) ec2.IVpc {
	vpc := ec2.Vpc_FromLookup(scope, jsii.String("Vpc"), &ec2.VpcLookupOptions{
		VpcId:     jsii.String(props.Id),
		IsDefault: jsii.Bool(props.IsDefault),
	})
	return vpc
}

func createEnvironmentFileBucketReadOnlyAccessPolicyStatement(bucket string) iam.PolicyStatement {
	policy := iam.NewPolicyStatement(
		&iam.PolicyStatementProps{
			Effect: iam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("s3:Get*"),
				jsii.String("s3:List*"),
			},
			Resources: &[]*string{
				jsii.String(bucket),
				jsii.String(bucket + "/*"),
			},
		},
	)

	return policy
}

func createEcrContainerRegistryReadonlyAccessPolicyStatement(registryNames []*string) iam.PolicyStatement {
	policy := iam.NewPolicyStatement(
		&iam.PolicyStatementProps{
			Effect: iam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("ecr:GetAuthorizationToken"),
				jsii.String("ecr:BatchCheckLayerAvailability"),
				jsii.String("ecr:GetDownloadUrlForLayer"),
				jsii.String("ecr:GetRepositoryPolicy"),
				jsii.String("ecr:DescribeRepositories"),
				jsii.String("ecr:ListImages"),
				jsii.String("ecr:DescribeImages"),
				jsii.String("ecr:BatchGetImage"),
				jsii.String("ecr:GetLifecyclePolicy"),
				jsii.String("ecr:GetLifecyclePolicyPreview"),
				jsii.String("ecr:ListTagsForResource"),
				jsii.String("ecr:DescribeImageScanFindings"),
			},
			Resources: &registryNames,
		},
	)
	return policy
}

func setupTaskContainerDefaultXrayPolciyStatement() iam.PolicyStatement {
	policy := iam.NewPolicyStatement(&iam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("xray:GetSamplingRules"),
			jsii.String("xray:GetSamplingStatisticSummaries"),
			jsii.String("xray:GetSamplingTargets"),
			jsii.String("xray:PutTelemetryRecords"),
			jsii.String("xray:PutTraceSegments"),
		},
		Effect: iam.Effect_ALLOW,
		// TODO: update resource section for OTEL policy
		Resources: &[]*string{jsii.String("*")},
	})

	return policy
}
