package ecs

import (
	"strconv"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	ec2 "github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	ecr "github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	ecs "github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	elb2 "github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	iam "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	cloudwatchlogs "github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	s3 "github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	servicediscovery "github.com/aws/aws-cdk-go/awscdk/v2/awsservicediscovery"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type LoadBalancedEc2ServiceProps struct {
	Cluster                     ClusterProps
	LogGroupName                string
	TaskDefinition              TaskDefinition
	EnableTracing               bool
	DesiredTaskCount            float64
	CapacityProviderStrategies  []string
	ServiceHealthPercent        ServiceHealthPercent
	IsServiceDiscoveryEnabled   bool
	ServiceDiscovery            ServiceDiscoveryOptions
	LoadBalancerTargetOptions   ecs.LoadBalancerTargetOptions
	RoutePriority               float64
	RoutePath                   string
	RoutePort                   float64
	RouteName                   string
	Host                        string
	IsLoadBalancerEnabled       bool
	LoadBalancerListenerArn     string
	LoadBalancerSecurityGroupId string
	LoadBalancerHealthCheckPath string
}

type ClusterProps struct {
	ClusterName    string
	Vpc            VpcProps
	SecurityGroups []ec2.ISecurityGroup
}

type VpcProps struct {
	Id        string
	IsDefault bool
}

type TaskDefinition struct {
	FamilyName string
	// Cpu                   string
	// MemoryInMiB           string
	NetworkMode           Networkmode
	EnvironmentFile       EnvironmentFile
	TaskPolicy            iam.PolicyDocument
	ApplicationContainers []ContainerDefinition
	RequiresVolume        bool
	Volumes               []Volume
}

type (
	Networkmode                string
	RegistryType               string
	LoadBalancerTargetProtocol string
)

type EnvironmentFile struct {
	BucketName string
	BucketArn  string
}

const (
	DEFAULT_LOG_RETENTION        cloudwatchlogs.RetentionDays = cloudwatchlogs.RetentionDays_TWO_WEEKS
	DEFAULT_DOCKER_VOLUME_DRIVER string                       = "rexray/ebs"
	DEFAULT_DOCKER_VOLUME_TYPE   string                       = "gp2"
	OTEL_CONTAINER_IMAGE         string                       = "amazon/aws-otel-collector:v0.25.0"
)

const (
	TASK_DEFINTION_NETWORK_MODE_BRIDGE   Networkmode     = "BRIDGE"
	TASK_DEFINTION_NETWORK_MODE_AWS_VPC  Networkmode     = "AWS_VPC"
	DEFAULT_TASK_DEFINITION_NETWORK_MODE ecs.NetworkMode = ecs.NetworkMode_BRIDGE
)

const (
	CONTAINER_DEFINITION_REGISTRY_AWS_ECR RegistryType = "ECR"
	CONTAINER_DEFINITION_REGISTRY_OTHERS  RegistryType = "OTHERS"
	DEFAULT_CONTAINER_DEFINITION_REGISTRY RegistryType = CONTAINER_DEFINITION_REGISTRY_AWS_ECR
)

const (
	LOAD_BALANCER_TARGET_PROTOCOL_TCP     LoadBalancerTargetProtocol = "TCP"
	LOAD_BALANCER_TARGET_PROTOCOL_UDP     LoadBalancerTargetProtocol = "UDP"
	DEFAULT_LOAD_BALANCER_TARGET_PROTOCOL ecs.Protocol               = ecs.Protocol_TCP
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

type ServiceDiscoveryOptions struct {
	NamespaceName string
	NamespaceId   string
	NamespaceArn  string
}

// type LoadBalancerTargetOptions struct {
// 	ContainerName string
// 	Port          float64
// 	Protocol      LoadBalancerTargetProtocol
// }

type loadBalancedEc2Service struct {
	constructs.Construct
	logGroup   cloudwatchlogs.LogGroup
	ec2Service ecs.Ec2Service
}

type LoadBalancedEc2Service interface {
	LogGroup() cloudwatchlogs.LogGroup
	Service() ecs.Ec2Service
}

func (s *loadBalancedEc2Service) Service() ecs.Ec2Service {
	return s.ec2Service
}

func (s *loadBalancedEc2Service) LogGroup() cloudwatchlogs.LogGroup {
	return s.logGroup
}

func NewLoadBalancedEc2Service(scope constructs.Construct, id *string, props *LoadBalancedEc2ServiceProps) LoadBalancedEc2Service {
	this := constructs.NewConstruct(scope, id)

	var taskPolicyDocument iam.PolicyDocument = nil
	if props.TaskDefinition.TaskPolicy != nil {

		taskPolicyDocument = props.TaskDefinition.TaskPolicy
		if props.EnableTracing {
			taskPolicyDocument.AddStatements(
				createTaskContainerDefaultXrayPolciyStatement(),
			)
		}
	} else {
		if props.EnableTracing {
			taskPolicyDocument = iam.NewPolicyDocument(&iam.PolicyDocumentProps{
				AssignSids: jsii.Bool(true),
				Statements: &[]iam.PolicyStatement{
					createTaskContainerDefaultXrayPolciyStatement(),
				},
			})
		}
	}

	var networkMode ecs.NetworkMode = DEFAULT_TASK_DEFINITION_NETWORK_MODE
	var loadBalancedServiceTargetType elb2.TargetType = elb2.TargetType_IP
	if props.TaskDefinition.NetworkMode == TASK_DEFINTION_NETWORK_MODE_AWS_VPC {
		networkMode = ecs.NetworkMode_AWS_VPC
		loadBalancedServiceTargetType = elb2.TargetType_IP
	} else if props.TaskDefinition.NetworkMode == TASK_DEFINTION_NETWORK_MODE_BRIDGE {
		networkMode = ecs.NetworkMode_BRIDGE
		loadBalancedServiceTargetType = elb2.TargetType_INSTANCE
	}
	// fmt.Println("loadBalancedServiceTargetType : ", loadBalancedServiceTargetType)

	var taskRole iam.Role = nil
	if taskPolicyDocument != nil {
		taskRole = iam.NewRole(this, jsii.String("TaskRole"), &iam.RoleProps{
			AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
			InlinePolicies: &map[string]iam.PolicyDocument{
				*jsii.String("DefaultPolicy"): taskPolicyDocument,
			},
		})
	}

	taskDef := ecs.NewEc2TaskDefinition(this, jsii.String("Ec2TaskDefinition"), &ecs.Ec2TaskDefinitionProps{
		Family: jsii.String(props.TaskDefinition.FamilyName),
		// Cpu:           jsii.String(props.TaskDefinition.Cpu),
		// MemoryMiB:     jsii.String(props.TaskDefinition.MemoryInMiB),
		// Compatibility: ecs.Compatibility_EC2,
		NetworkMode: networkMode,
		ExecutionRole: iam.NewRole(this, jsii.String("ExecutionRole"), &iam.RoleProps{
			AssumedBy: iam.NewServicePrincipal(jsii.String("ecs-tasks."+*awscdk.Aws_URL_SUFFIX()), &iam.ServicePrincipalOpts{}),
			InlinePolicies: &map[string]iam.PolicyDocument{
				*jsii.String("DefaultPolicy"): iam.NewPolicyDocument(
					&iam.PolicyDocumentProps{
						AssignSids: jsii.Bool(true),
						Statements: &[]iam.PolicyStatement{
							iam.NewPolicyStatement(
								&iam.PolicyStatementProps{
									Actions: &[]*string{
										jsii.String("s3:GetBucketLocation"),
									},
									Effect: iam.Effect_ALLOW,
									Resources: &[]*string{
										&props.TaskDefinition.EnvironmentFile.BucketArn,
									},
								},
							),
						},
					},
				),
			},
		}),
		TaskRole: taskRole,
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

	// Creates a CloudWatch Log Group for each service
	logGroup := cloudwatchlogs.NewLogGroup(this, jsii.String("LogGroup"), &cloudwatchlogs.LogGroupProps{
		LogGroupName: jsii.String(props.LogGroupName),
		Retention:    DEFAULT_LOG_RETENTION,
	})
	logGroup.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	for index, containerDef := range props.TaskDefinition.ApplicationContainers {
		// update task definition with statements providing container the acces to specific environment files in th S3 bucket
		taskDef.AddToExecutionRolePolicy(
			createEnvironmentFileObjectReadOnlyAccessPolicyStatement(
				props.TaskDefinition.EnvironmentFile.BucketArn,
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
				jsii.String(props.TaskDefinition.EnvironmentFile.BucketName),
			),
			logGroup,
			props.EnableTracing,
		)
		cd.AddMountPoints(convertContainerVolumeMountPoints(containerDef.VolumeMountPoint)...)
	}

	// if props.EnableTracing {
	// 	ecs.NewContainerDefinition(scope, jsii.String("OtelContainerDefinition"), &ecs.ContainerDefinitionProps{
	// 		TaskDefinition: taskDef,
	// 		ContainerName:  jsii.String("Otel"),
	// 		Image:          ecs.ContainerImage_FromRegistry(jsii.String(OTEL_CONTAINER_IMAGE), &ecs.RepositoryImageProps{}),
	// 		Cpu:            jsii.Number(256),
	// 		MemoryLimitMiB: jsii.Number(512),
	// 		Logging:        setupContianerAwsLogDriver(logGroup, "Otel"),
	// 		Command: &[]*string{
	// 			jsii.String("--config=/etc/ecs/ecs-default-config.yaml"),
	// 		},
	// 		PortMappings: &[]*ecs.PortMapping{
	// 			{
	// 				ContainerPort: jsii.Number(2000),
	// 				HostPort:      jsii.Number(2000),
	// 				Protocol:      ecs.Protocol_UDP,
	// 			},
	// 			{
	// 				ContainerPort: jsii.Number(4317),
	// 				HostPort:      jsii.Number(4317),
	// 				Protocol:      ecs.Protocol_TCP,
	// 			},
	// 			{
	// 				ContainerPort: jsii.Number(8125),
	// 				HostPort:      jsii.Number(8125),
	// 				Protocol:      ecs.Protocol_UDP,
	// 			},
	// 		},
	// 	})
	// }

	var capacityProviderStrategies []*ecs.CapacityProviderStrategy = []*ecs.CapacityProviderStrategy{}
	for _, cps := range props.CapacityProviderStrategies {
		capacityProviderStrategy := createServiceCapacityProviderStrategy(cps)
		capacityProviderStrategies = append(capacityProviderStrategies, &capacityProviderStrategy)
	}

	vpc := lookupVpc(this, id, &props.Cluster.Vpc)
	var cmOpts *ecs.CloudMapOptions = nil
	if props.IsServiceDiscoveryEnabled {
		// fmt.Println("Entering ServiceDiscovery configuration")
		cmOpts = &ecs.CloudMapOptions{
			DnsTtl:            awscdk.Duration_Minutes(jsii.Number(1)),
			DnsRecordType:     servicediscovery.DnsRecordType_A,
			ContainerPort:     jsii.Number(props.RoutePort),
			Name:              jsii.String(props.RouteName),
			CloudMapNamespace: getCloudMapNamespaceService(this, props.ServiceDiscovery),
		}
	}
	ec2Service := ecs.NewEc2Service(this, jsii.String("Ec2Service"), &ecs.Ec2ServiceProps{
		Cluster: ecs.Cluster_FromClusterAttributes(this, jsii.String("Cluster"), &ecs.ClusterAttributes{
			ClusterName:    jsii.String(props.Cluster.ClusterName),
			Vpc:            vpc,
			SecurityGroups: &props.Cluster.SecurityGroups,
		}),
		CapacityProviderStrategies: &capacityProviderStrategies,
		TaskDefinition:             taskDef,
		DesiredCount:               &props.DesiredTaskCount,
		CircuitBreaker: &ecs.DeploymentCircuitBreaker{
			Rollback: jsii.Bool(true),
		},
		PlacementStrategies: &[]ecs.PlacementStrategy{
			ecs.PlacementStrategy_PackedByMemory(),
		},
		CloudMapOptions:      cmOpts,
		PropagateTags:        ecs.PropagatedTagSource_SERVICE,
		EnableECSManagedTags: jsii.Bool(true),
	})

	// stc := ec2Service.AutoScaleTaskCount(&awsapplicationautoscaling.EnableScalingProps{
	// 	MaxCapacity: jsii.Number(10),
	// 	MinCapacity: jsii.Number(1),
	// })

	// stc.ScaleOnMemoryUtilization(jsii.String(""),&ecs.MemoryUtilizationScalingProps{

	// })

	// var serviceTargets []elb2.IApplicationLoadBalancerTarget = []elb2.IApplicationLoadBalancerTarget{}

	// for _, t := range props.LoadBalancerTargetOptions {

	// 	var protocol ecs.Protocol = DEFAULT_LOAD_BALANCER_TARGET_PROTOCOL

	// 	if t.Protocol == LOAD_BALANCER_TARGET_PROTOCOL_TCP {
	// 		protocol = ecs.Protocol_TCP
	// 	} else {
	// 		protocol = ecs.Protocol_UDP
	// 	}

	// 	serviceTargets = append(serviceTargets, ec2Service.LoadBalancerTarget(
	// 		&ecs.LoadBalancerTargetOptions{
	// 			ContainerName: jsii.String(t.ContainerName),
	// 			ContainerPort: jsii.Number(t.Port),
	// 			Protocol:      protocol,
	// 		},
	// 	))
	// }
	// fmt.Println("Number of service targets: ", len(serviceTargets))
	// fmt.Println("Service targets: ", serviceTargets)

	// appTg := elb2.NewApplicationTargetGroup(this, jsii.String("ApplicationTargetGroup"), &elb2.ApplicationTargetGroupProps{
	// 	HealthCheck: &elb2.HealthCheck{
	// 		Enabled:          jsii.Bool(true),
	// 		HealthyHttpCodes: jsii.String("200"),
	// 		Path:             jsii.String("/"),
	// 		Interval:         awscdk.Duration_Seconds(jsii.Number(30)),
	// 	},
	// 	TargetGroupName: jsii.String("LoadBalancedTg"),
	// 	TargetType:      loadBalancedServiceTargetType,
	// 	Vpc:             vpc,
	// 	Protocol:        elb2.ApplicationProtocol_HTTP,
	// 	Targets: &[]elb2.IApplicationLoadBalancerTarget{
	// 		ec2Service.LoadBalancerTarget(&ecs.LoadBalancerTargetOptions{
	// 			ContainerName: jsii.String("demo-app"),
	// 			ContainerPort: jsii.Number(80),
	// 			Protocol:      ecs.Protocol_TCP,
	// 		}),
	// 	},
	// })

	// elb2.NewApplicationListenerRule(this, jsii.String("ALBListenerRule"), &elb2.ApplicationListenerRuleProps{
	// 	Priority: jsii.Number(props.RoutePriority),
	// 	Action:   elb2.ListenerAction_Forward(&[]elb2.IApplicationTargetGroup{appTg}, &elb2.ForwardOptions{}),
	// 	Conditions: &[]elb2.ListenerCondition{
	// 		elb2.ListenerCondition_HostHeaders(&[]*string{jsii.String(props.Host)}),
	// 		elb2.ListenerCondition_PathPatterns(&[]*string{jsii.String(props.RoutePath)}),
	// 	},
	// 	Listener: elb2.ApplicationListener_FromApplicationListenerAttributes(this, jsii.String("ALBListener"), &elb2.ApplicationListenerAttributes{
	// 		ListenerArn:   jsii.String(props.LoadBalancerListenerArn),
	// 		SecurityGroup: ec2.SecurityGroup_FromLookupById(this, jsii.String("ALBSecurityGroup"), jsii.String(props.LoadBalancerSecurityGroupId)),
	// 	}),
	// })

	if props.IsLoadBalancerEnabled {
		ecsServiceTargetGroup := elb2.NewApplicationTargetGroup(this, jsii.String("ApplicationTargetGroup"), &elb2.ApplicationTargetGroupProps{
			// TargetGroupName: jsii.String("LoadBalancedTargetGroup"),
			HealthCheck: &elb2.HealthCheck{
				Enabled:          jsii.Bool(true),
				HealthyHttpCodes: jsii.String("200"),
				Path:             jsii.String(props.LoadBalancerHealthCheckPath),
				Interval:         awscdk.Duration_Seconds(jsii.Number(30)),
			},
			TargetType: loadBalancedServiceTargetType,
			Vpc:        vpc,
			Protocol:   elb2.ApplicationProtocol_HTTP,
			Targets: &[]elb2.IApplicationLoadBalancerTarget{
				ec2Service.LoadBalancerTarget(&props.LoadBalancerTargetOptions),
			},
		})

		elb2.NewApplicationListenerRule(this, jsii.String("ALBListenerRule"), &elb2.ApplicationListenerRuleProps{
			Priority: jsii.Number(props.RoutePriority),
			Action:   elb2.ListenerAction_Forward(&[]elb2.IApplicationTargetGroup{ecsServiceTargetGroup}, &elb2.ForwardOptions{}),
			Conditions: &[]elb2.ListenerCondition{
				elb2.ListenerCondition_HostHeaders(jsii.Strings(props.Host)),
				elb2.ListenerCondition_PathPatterns(jsii.Strings(props.RoutePath)),
			},
			Listener: elb2.ApplicationListener_FromApplicationListenerAttributes(this, jsii.String("ALBListener"), &elb2.ApplicationListenerAttributes{
				ListenerArn:   jsii.String(props.LoadBalancerListenerArn),
				SecurityGroup: ec2.SecurityGroup_FromLookupById(this, jsii.String("ALBSecurityGroup"), jsii.String(props.LoadBalancerSecurityGroupId)),
			}),
		})
	}

	return &loadBalancedEc2Service{this, logGroup, ec2Service}
}

func configureContainerToTaskDefinition(scope constructs.Construct, id string, containerDef ContainerDefinition, taskDef ecs.TaskDefinition, taskDefEnvFileBucket s3.IBucket, logGroup cloudwatchlogs.ILogGroup, tracingEnabled bool) ecs.ContainerDefinition {
	cd := ecs.NewContainerDefinition(scope, jsii.String(id), &ecs.ContainerDefinitionProps{
		TaskDefinition: taskDef,
		ContainerName:  &containerDef.ContainerName,
		Command:        convertContainerCommands(containerDef.Commands),
		EntryPoint:     convertContainerEntryPointCommands(containerDef.EntryPointCommands),
		Essential:      jsii.Bool(containerDef.IsEssential),
		Image:          configureContainerImage(scope, containerDef.RegistryType, containerDef.Image, containerDef.ImageTag),
		Cpu:            jsii.Number(containerDef.Cpu),
		MemoryLimitMiB: jsii.Number(containerDef.Memory),
		EnvironmentFiles: &[]ecs.EnvironmentFile{
			ecs.AssetEnvironmentFile_FromBucket(taskDefEnvFileBucket, jsii.String(containerDef.EnvironmentFileObjectKey), nil),
		},
		Logging:      setupContianerAwsLogDriver(logGroup, containerDef.ContainerName),
		PortMappings: convertContainerPortMappings(containerDef.PortMappings),
	})

	if tracingEnabled {
		otelContainerDef := ecs.NewContainerDefinition(scope, jsii.String("OtelContainerDefinition"), &ecs.ContainerDefinitionProps{
			TaskDefinition: taskDef,
			ContainerName:  jsii.String("otel-xray"),
			Image:          ecs.ContainerImage_FromRegistry(jsii.String(OTEL_CONTAINER_IMAGE), &ecs.RepositoryImageProps{}),
			Cpu:            jsii.Number(256),
			MemoryLimitMiB: jsii.Number(256),
			Logging:        setupContianerAwsLogDriver(logGroup, "Otel"),
			Command: &[]*string{
				jsii.String("--config=/etc/ecs/ecs-default-config.yaml"),
			},
			PortMappings: &[]*ecs.PortMapping{
				{
					ContainerPort: jsii.Number(2000),
					HostPort:      jsii.Number(2000),
					Protocol:      ecs.Protocol_UDP,
				},
				{
					ContainerPort: jsii.Number(4317),
					HostPort:      jsii.Number(4317),
					Protocol:      ecs.Protocol_TCP,
				},
				{
					ContainerPort: jsii.Number(8125),
					HostPort:      jsii.Number(8125),
					Protocol:      ecs.Protocol_UDP,
				},
			},
		})
		cd.AddLink(otelContainerDef, jsii.String("otel-xray"))
		cd.AddContainerDependencies(&ecs.ContainerDependency{
			Condition: ecs.ContainerDependencyCondition_START,
			Container: otelContainerDef,
		})
	}

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
		return ecs.ContainerImage_FromRegistry(jsii.String(image+":"+tag), &ecs.RepositoryImageProps{})
	}
}

func setupContianerAwsLogDriver(logGroup cloudwatchlogs.ILogGroup, prefix string) ecs.LogDriver {
	logDriver := ecs.AwsLogDriver_AwsLogs(&ecs.AwsLogDriverProps{
		LogGroup:     logGroup,
		StreamPrefix: jsii.String(prefix),
		// LogRetention: DEFAULT_LOG_RETENTION,
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

func createServiceCapacityProviderStrategy(name string) ecs.CapacityProviderStrategy {
	capacityProviderStrategy := ecs.CapacityProviderStrategy{
		CapacityProvider: jsii.String(name),
		Weight:           jsii.Number(1),
	}

	return capacityProviderStrategy
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
				// jsii.String(bucket),
				jsii.String(bucket + "/" + key),
			},
		},
	)

	return policy
}

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

func getCloudMapNamespaceService(scope constructs.Construct, sd ServiceDiscoveryOptions) servicediscovery.IPrivateDnsNamespace {
	privateNamespace := servicediscovery.PrivateDnsNamespace_FromPrivateDnsNamespaceAttributes(
		scope, jsii.String("CloudMapNamespace"), &servicediscovery.PrivateDnsNamespaceAttributes{
			NamespaceArn:  jsii.String(sd.NamespaceArn),
			NamespaceId:   jsii.String(sd.NamespaceId),
			NamespaceName: jsii.String(sd.NamespaceName),
		},
	)
	return privateNamespace
}
