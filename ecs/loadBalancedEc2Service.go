package ecs

import (
	"strconv"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	ec2 "github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	ecr "github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	ecs "github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	iam "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	cloudwatchlogs "github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	s3 "github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type LoadBalancedEc2ServiceProps struct {
	Cluster                    ClusterProps
	LogGroupName               string
	TaskDefinition             TaskDefinition
	DesiredTaskCount           float64
	CapacityProviderStrategies []ecs.CapacityProviderStrategy
	ServiceHealthPercent       ServiceHealthPercent
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
	NetworkMode           Networkmode
	EnvironmentFileBucket string
	TaskPolicy            iam.PolicyDocument
	ApplicationContainers []ContainerDefinition
	RequiresVolume        bool
	Volumes               []Volume
}

type Networkmode string

type RegistryType string

const (
	TASK_DEFINTION_NETWORK_MODE_BRIDGE    Networkmode                  = "BRIDGE"
	TASK_DEFINTION_NETWORK_MODE_AWS_VPC   Networkmode                  = "AWS_VPC"
	DEFAULT_TASK_DEFINITION_NETWORK_MODE  ecs.NetworkMode              = ecs.NetworkMode_BRIDGE
	CONTAINER_DEFINITION_REGISTRY_AWS_ECR RegistryType                 = "ECR"
	CONTAINER_DEFINITION_REGISTRY_OTHERS  RegistryType                 = "OTHERS"
	DEFAULT_LOG_RETENTION                 cloudwatchlogs.RetentionDays = cloudwatchlogs.RetentionDays_TWO_WEEKS
	DEFAULT_DOCKER_VOLUME_DRIVER          string                       = "rexray/ebs"
	DEFAULT_DOCKER_VOLUME_TYPE            string                       = "gp2"
)

type ContainerDefinition struct {
	ContainerName            string
	Image                    string
	RegistryType             RegistryType
	ImageTag                 string
	IsEssential              bool
	Commands                 []string
	EntryPointCommands       []string
	Cpu                      float64
	Memory                   float64
	PortMappings             []ecs.PortMapping
	EnvironmentFileObjectKey string
	VolumeMountPoint         []ecs.MountPoint
}

type Volume struct {
	Name string
	Size string
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
		createTaskContainerDefaultXrayPolciyStatement(),
	)

	logGroup := cloudwatchlogs.LogGroup_FromLogGroupName(this, jsii.String("LogGroup"), jsii.String(props.LogGroupName))

	var networkMode ecs.NetworkMode = DEFAULT_TASK_DEFINITION_NETWORK_MODE
	if props.TaskDefinition.NetworkMode == TASK_DEFINTION_NETWORK_MODE_AWS_VPC {
		networkMode = ecs.NetworkMode_AWS_VPC
	} else if props.TaskDefinition.NetworkMode == TASK_DEFINTION_NETWORK_MODE_BRIDGE {
		networkMode = ecs.NetworkMode_BRIDGE
	}

	taskDef := ecs.NewTaskDefinition(this, jsii.String("Ec2TaskDefinition"), &ecs.TaskDefinitionProps{
		Family:        jsii.String(props.TaskDefinition.FamilyName),
		Cpu:           jsii.String(props.TaskDefinition.Cpu),
		MemoryMiB:     jsii.String(props.TaskDefinition.MemoryInMiB),
		Compatibility: ecs.Compatibility_EC2,
		NetworkMode:   networkMode,
		ExecutionRole: iam.NewRole(this, jsii.String("ExecutionRole"), &iam.RoleProps{
			AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
			InlinePolicies: &map[string]iam.PolicyDocument{
				*jsii.String("DefaultPolicy"): iam.NewPolicyDocument(
					&iam.PolicyDocumentProps{
						AssignSids: jsii.Bool(true),
						Statements: &[]iam.PolicyStatement{
							// createEnvironmentFileBucketReadOnlyAccessPolicyStatement(props.TaskDefinition.EnvironmentFileBucket),
							// createEcrContainerRegistryReadonlyAccessPolicyStatement(ecrRepositories),
							iam.NewPolicyStatement(
								&iam.PolicyStatementProps{
									Actions: &[]*string{
										jsii.String("s3:GetBucketLocation"),
									},
									Effect: iam.Effect_ALLOW,
									Resources: &[]*string{
										&props.TaskDefinition.EnvironmentFileBucket,
									},
								},
							),
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

	if props.TaskDefinition.RequiresVolume {
		for _, volume := range props.TaskDefinition.Volumes {
			var vol ecs.Volume = ecs.Volume{
				Name: jsii.String(volume.Name),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Driver:        jsii.String(DEFAULT_DOCKER_VOLUME_DRIVER),
					Scope:         ecs.Scope_SHARED,
					Autoprovision: jsii.Bool(true),
					DriverOpts: &map[string]*string{
						"volumetype": jsii.String(DEFAULT_DOCKER_VOLUME_TYPE),
						"size":       jsii.String(volume.Size),
					},
				},
			}
			taskDef.AddVolume(&vol)
		}
	}

	for index, containerDef := range props.TaskDefinition.ApplicationContainers {
		// update task definition with statements providing container the acces to specific environment files in th S3 bucket
		taskDef.AddToExecutionRolePolicy(
			createEnvironmentFileObjectReadOnlyAccessPolicyStatement(
				props.TaskDefinition.EnvironmentFileBucket,
				containerDef.EnvironmentFileObjectKey),
		)
		// creates container definition for the task definition
		cd := configureContainerToTaskDefinition(
			this,
			"Container"+strconv.FormatInt(int64(index), 10),
			containerDef,
			taskDef,
			s3.Bucket_FromBucketName(
				this,
				jsii.String("EnvironmentFileBucket"),
				jsii.String(props.TaskDefinition.EnvironmentFileBucket),
			),
			logGroup,
		)
		cd.AddMountPoints(convertContainerVolumeMountPoints(containerDef.VolumeMountPoint)...)
	}

	ec2Service := ecs.NewEc2Service(this, jsii.String("Ec2Service"), &ecs.Ec2ServiceProps{
		Cluster: ecs.Cluster_FromClusterAttributes(this, jsii.String("Cluster"), &ecs.ClusterAttributes{
			ClusterName:    jsii.String(props.Cluster.ClusterName),
			Vpc:            lookupVpc(this, id, &props.Cluster.Vpc),
			SecurityGroups: &props.Cluster.Vpc.SecurityGroups,
		}),
		// CapacityProviderStrategies: &[]*ecs.CapacityProviderStrategy{
		// 	&ecs.CapacityProviderStrategy{
		// 		CapacityProvider: jsii.String(""),
		// 		Weight:           jsii.Number(1),
		// 	},
		// },
		TaskDefinition: taskDef,
		DesiredCount:   &props.DesiredTaskCount,
		CircuitBreaker: &ecs.DeploymentCircuitBreaker{
			Rollback: jsii.Bool(true),
		},
		PlacementStrategies: &[]ecs.PlacementStrategy{
			ecs.PlacementStrategy_PackedByMemory(),
		},
	})

	return &loadBalancedEc2Service{this, ec2Service}
}

func configureContainerToTaskDefinition(scope constructs.Construct, id string, containerDef ContainerDefinition, taskDef ecs.TaskDefinition, taskDefEnvFileBucket s3.IBucket, logGroup cloudwatchlogs.ILogGroup) ecs.ContainerDefinition {
	cd := ecs.NewContainerDefinition(scope, jsii.String(id), &ecs.ContainerDefinitionProps{
		TaskDefinition: taskDef,
		ContainerName:  &containerDef.ContainerName,
		Command:        convertContainerCommands(containerDef.Commands),
		EntryPoint:     convertContainerEntryPointCommands(containerDef.EntryPointCommands),
		Essential:      jsii.Bool(containerDef.IsEssential),
		Image:          configureContainerImage(scope, containerDef.RegistryType, containerDef.Image, containerDef.ImageTag),
		Cpu:            &containerDef.Cpu,
		MemoryLimitMiB: &containerDef.Memory,
		EnvironmentFiles: &[]ecs.EnvironmentFile{
			ecs.AssetEnvironmentFile_FromBucket(taskDefEnvFileBucket, jsii.String(containerDef.EnvironmentFileObjectKey), nil),
		},
		Logging:      setupContianerAwsLogDriver(logGroup, containerDef.ContainerName),
		PortMappings: convertContainerPortMappings(containerDef.PortMappings),
	})

	return cd
}

func convertContainerCommands(cmds []string) *[]*string {
	commands := []*string{}
	for _, cmd := range cmds {
		commands = append(commands, jsii.String(cmd))
	}
	return &commands
}

func convertContainerEntryPointCommands(cmds []string) *[]*string {
	entryPointCmds := []*string{}
	for _, cmd := range cmds {
		entryPointCmds = append(entryPointCmds, jsii.String(cmd))
	}
	return &entryPointCmds
}

func convertContainerPortMappings(pm []ecs.PortMapping) *[]*ecs.PortMapping {
	portMapping := []*ecs.PortMapping{}
	for _, mapping := range pm {
		portMapping = append(portMapping, &mapping)
	}
	return &portMapping

}

func convertContainerVolumeMountPoints(pm []ecs.MountPoint) []*ecs.MountPoint {
	mountPoints := []*ecs.MountPoint{}
	for _, mount := range pm {
		mountPoints = append(mountPoints, &mount)
	}
	return mountPoints
}

func configureContainerImage(scope constructs.Construct, registryType RegistryType, image string, tag string) ecs.ContainerImage {
	if registryType == CONTAINER_DEFINITION_REGISTRY_AWS_ECR {
		return ecs.ContainerImage_FromEcrRepository(ecr.Repository_FromRepositoryName(scope, jsii.String("EcrRepository"), jsii.String(image)), jsii.String(tag))
	} else {
		return ecs.ContainerImage_FromRegistry(jsii.String(image+tag), &ecs.RepositoryImageProps{})
	}
}

func setupContianerAwsLogDriver(logGroup cloudwatchlogs.ILogGroup, prefix string) ecs.LogDriver {
	logDriver := ecs.AwsLogDriver_AwsLogs(&ecs.AwsLogDriverProps{
		LogGroup:     logGroup,
		StreamPrefix: jsii.String(prefix),
		LogRetention: DEFAULT_LOG_RETENTION,
	})
	return logDriver
}

func lookupVpc(scope constructs.Construct, id *string, props *VpcProps) ec2.IVpc {
	vpc := ec2.Vpc_FromLookup(scope, jsii.String("Vpc"), &ec2.VpcLookupOptions{
		VpcId:     jsii.String(props.Id),
		IsDefault: jsii.Bool(props.IsDefault),
	})
	return vpc
}

func createEnvironmentFileObjectReadOnlyAccessPolicyStatement(bucket string, key string) iam.PolicyStatement {

	policy := iam.NewPolicyStatement(
		&iam.PolicyStatementProps{
			Effect: iam.Effect_ALLOW,
			Actions: &[]*string{
				// jsii.String("s3:GetBucketLocation"),
				jsii.String("s3:GetObject"),
			},
			Resources: &[]*string{
				jsii.String(bucket),
				jsii.String(bucket + "/" + key),
			},
		},
	)

	return policy
}

// func createEcrContainerRegistryReadonlyAccessPolicyStatement(registryNames []*string) iam.PolicyStatement {
// 	policy := iam.NewPolicyStatement(
// 		&iam.PolicyStatementProps{
// 			Effect: iam.Effect_ALLOW,
// 			Actions: &[]*string{
// 				jsii.String("ecr:GetAuthorizationToken"),
// 				jsii.String("ecr:BatchCheckLayerAvailability"),
// 				jsii.String("ecr:GetDownloadUrlForLayer"),
// 				jsii.String("ecr:GetRepositoryPolicy"),
// 				jsii.String("ecr:DescribeRepositories"),
// 				jsii.String("ecr:ListImages"),
// 				jsii.String("ecr:DescribeImages"),
// 				jsii.String("ecr:BatchGetImage"),
// 				jsii.String("ecr:GetLifecyclePolicy"),
// 				jsii.String("ecr:GetLifecyclePolicyPreview"),
// 				jsii.String("ecr:ListTagsForResource"),
// 				jsii.String("ecr:DescribeImageScanFindings"),
// 			},
// 			Resources: &registryNames,
// 		},
// 	)
// 	return policy
// }

func createTaskContainerDefaultXrayPolciyStatement() iam.PolicyStatement {
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
