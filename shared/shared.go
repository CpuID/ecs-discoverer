package shared

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

// Verify that the ECS cluster exists.
func VerifyClusterExists(ecs_obj *ecs.ECS, cluster string) {
	params := &ecs.DescribeClustersInput{
		Clusters: []*string{
			aws.String(cluster),
		},
	}
	clusters, err := ecs_obj.DescribeClusters(params)

	if err != nil {
		fmt.Println("Cannot verify if ECS cluster exists:")
		formatAwsError(err)
		os.Exit(1)
	}
	if len(clusters.Clusters) == 0 {
		fmt.Printf("Error: ECS Cluster '%s' does not exist, cannot proceed.\n", cluster)
		os.Exit(1)
	}
	if len(clusters.Clusters) != 1 {
		fmt.Printf("Error: Unexpected number of ECS Clusters returned when searching for '%s'. Received: %+v\n", cluster, clusters.Clusters)
		os.Exit(1)
	}
}

// Verify that the ECS service exists.
func VerifyServiceExists(ecs_obj *ecs.ECS, cluster string, service string) {
	params := &ecs.DescribeServicesInput{
		Cluster: &cluster,
		Services: []*string{ // Required
			aws.String(service), // Required
		},
	}
	_, err := ecs_obj.DescribeServices(params)

	if err != nil {
		fmt.Println("Cannot verify if ECS service exists:")
		formatAwsError(err)
		os.Exit(1)
	}
}

func GetContainerInstanceArnsForService(ecs_obj *ecs.ECS, cluster string, service string, local_container_instance_arn string, debug bool) []string {
	// Fetch a task list based on the service name.
	list_tasks_params := &ecs.ListTasksInput{
		Cluster:     &cluster,
		ServiceName: &service,
	}
	list_tasks_resp, list_tasks_err := ecs_obj.ListTasks(list_tasks_params)

	if list_tasks_err != nil {
		fmt.Println("Cannot retrieve ECS task list:")
		formatAwsError(list_tasks_err)
		os.Exit(1)
	}

	if len(list_tasks_resp.TaskArns) <= 0 {
		fmt.Println("No ECS tasks found with specified filter - cluster: ", cluster, ", service:", service)
		os.Exit(1)
	}

	// Describe the tasks retrieved above.
	describe_tasks_params := &ecs.DescribeTasksInput{
		Cluster: &cluster,
		Tasks:   list_tasks_resp.TaskArns,
	}
	describe_tasks_resp, describe_tasks_err := ecs_obj.DescribeTasks(describe_tasks_params)

	if describe_tasks_err != nil {
		fmt.Println("Cannot retrieve ECS task details:")
		formatAwsError(describe_tasks_err)
		os.Exit(1)
	}

	if len(describe_tasks_resp.Tasks) <= 0 {
		fmt.Println("No ECS task details found with specified filter - tasks:", strings.Join(aws.StringValueSlice(list_tasks_resp.TaskArns), ", "))
		os.Exit(1)
	}

	var result []string
	for _, value := range describe_tasks_resp.Tasks {
		if *value.LastStatus == "RUNNING" && *value.ContainerInstanceArn != local_container_instance_arn {
			result = append(result, *value.ContainerInstanceArn)
		} else {
			if debug == true {
				fmt.Println(*value.ContainerInstanceArn, "is not in a RUNNING state, or is this instance (we dont return ourself). Excluded from results.")
			}
		}
	}

	if len(result) == 0 {
		fmt.Println("No ECS task results found in RUNNING state, no ECS container instances to return.")
		os.Exit(1)
	}
	return result
}

func GetEc2InstanceIdsFromContainerInstances(ecs_obj *ecs.ECS, cluster string, container_instances []string, debug bool) []string {
	params := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(cluster),
		ContainerInstances: aws.StringSlice(container_instances),
	}
	resp, err := ecs_obj.DescribeContainerInstances(params)

	if err != nil {
		fmt.Println("Cannot retrieve ECS container instance information:")
		formatAwsError(err)
		os.Exit(1)
	}

	if len(resp.ContainerInstances) <= 0 {
		fmt.Println("No ECS container instances found with specified filter - cluster:", cluster, "- instances:", strings.Join(container_instances, ", "))
		os.Exit(1)
	}

	var result []string
	for _, value := range resp.ContainerInstances {
		if *value.Status == "ACTIVE" {
			result = append(result, *value.Ec2InstanceId)
		} else {
			if debug == true {
				fmt.Println(*value.Ec2InstanceId, "is not in an ACTIVE state, excluded from results.")
			}
		}
	}

	if len(result) == 0 {
		fmt.Println("No running ECS container instances found in result set, cannot proceed.")
		os.Exit(1)
	}
	return result
}

func GetEc2PrivateIpsFromInstanceIds(ec2_obj *ec2.EC2, instance_ids []string, debug bool) []string {
	params := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(instance_ids),
	}
	resp, err := ec2_obj.DescribeInstances(params)

	if err != nil {
		fmt.Println("Cannot retrieve EC2 instance information:")
		formatAwsError(err)
		os.Exit(1)
	}

	if len(resp.Reservations) <= 0 {
		fmt.Println("No EC2 instances found (Reservations.*) with specified Instance IDs filter: ", strings.Join(instance_ids, ", "))
		os.Exit(1)
	}
	if len(resp.Reservations[0].Instances) <= 0 {
		fmt.Println("No EC2 instances found (Reservations[0].* with specified Instance IDs filter: ", strings.Join(instance_ids, ", "))
		os.Exit(1)
	}

	var result []string
	for idx, _ := range resp.Reservations {
		for _, value := range resp.Reservations[idx].Instances {
			if *value.State.Name == "running" {
				result = append(result, *value.PrivateIpAddress)
			} else {
				if debug == true {
					fmt.Println(*value.InstanceId, "is not in a running state, excluded from results.")
				}
			}
		}
	}

	if len(result) == 0 {
		fmt.Println("No running EC2 instances found in result set, cannot proceed.")
		os.Exit(1)
	}
	return result
}
