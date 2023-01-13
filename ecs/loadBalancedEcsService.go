package ecs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	ec2 "github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	ecs "github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	iam "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// type NetworkMode string

// const (
// 	BRIDGE  NetworkMode = "BRIDGE"
// 	AWS_VPC NetworkMode = "AWS_VPC"
// )

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
	FamilyName  string
	Cpu         string
	MemoryInMiB string
	// NetworkMode NetworkMode
	TaskPolicyDocument iam.PolicyDocument
}

type ServiceHealthPercent struct {
}

type loadBalancedEc2Service struct {
	constructs.Construct
	ec2Service ecs.Ec2Service
}

type LoadBalancedEc2Service interface {
	// constructs.Construct
	Service() ecs.Ec2Service
}

func (s *loadBalancedEc2Service) Service() ecs.Ec2Service {
	return s.ec2Service
}

func NewLoadBalancedEc2Service(scope constructs.Construct, id *string, props *LoadBalancedEc2ServiceProps) LoadBalancedEc2Service {
	this := constructs.NewConstruct(scope, id)

	taskPolicyDocument := props.TaskDefinition.TaskPolicyDocument
	taskPolicyDocument.AddStatements(
		iam.NewPolicyStatement(&iam.PolicyStatementProps{
			Actions: &[]*string{
				jsii.String("xray:GetSamplingRules"),
				jsii.String("xray:GetSamplingStatisticSummaries"),
				jsii.String("xray:GetSamplingTargets"),
				jsii.String("xray:PutTelemetryRecords"),
				jsii.String("xray:PutTraceSegments"),
			},
			Effect: iam.Effect_ALLOW,
			// TODO: update resource for OTEL policy
			Resources: &[]*string{jsii.String("*")},
		}),
	)

	ec2Service := ecs.NewEc2Service(this, jsii.String("Ec2Service"), &ecs.Ec2ServiceProps{
		Cluster: ecs.Cluster_FromClusterAttributes(this, jsii.String("Cluster"), &ecs.ClusterAttributes{
			ClusterName:    jsii.String(props.Cluster.ClusterName),
			Vpc:            lookupVpc(this, id, &props.Cluster.Vpc),
			SecurityGroups: &props.Cluster.Vpc.SecurityGroups,
		}),
		TaskDefinition: ecs.NewTaskDefinition(this, jsii.String("Ec2TaskDefinition"), &ecs.TaskDefinitionProps{
			Family:        jsii.String(props.TaskDefinition.FamilyName),
			Cpu:           jsii.String(props.TaskDefinition.Cpu),
			MemoryMiB:     jsii.String(props.TaskDefinition.MemoryInMiB),
			Compatibility: ecs.Compatibility_EC2,
			// NetworkMode:   configureNetworkMode(props.TaskDefinition.NetworkMode),
			NetworkMode: ecs.NetworkMode_BRIDGE,
			ExecutionRole: iam.NewRole(this, jsii.String("ExecutionRole"), &iam.RoleProps{
				AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
				ManagedPolicies: &[]iam.IManagedPolicy{
					iam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AmazonS3ReadOnlyAccess")),
				},
			}),
			TaskRole: iam.NewRole(this, jsii.String("TaskRole"), &iam.RoleProps{
				AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
				InlinePolicies: &map[string]iam.PolicyDocument{
					*jsii.String("DefaultPolicy"): taskPolicyDocument,
				},
			}),
		}),
		DesiredCount: &props.DesiredTaskCount,
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

// func configureNetworkMode(networkMode NetworkMode) ecs.NetworkMode {
// 	var nw ecs.NetworkMode
// 	switch networkMode {
// 	case BRIDGE:
// 		nw = ecs.NetworkMode_BRIDGE
// 	case AWS_VPC:
// 		nw = ecs.NetworkMode_AWS_VPC
// 	}
// 	return nw
// }
