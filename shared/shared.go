package shared

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

// Save some repetition, formatting the output of these.
func FormatAwsError(err error) string {
	var result string
	if awsErr, ok := err.(awserr.Error); ok {
		// Generic AWS Error with Code, Message, and original error (if any)
		result = fmt.Sprintf("%s %s %s", awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			// A service error occurred
			result = fmt.Sprintf("%s\n%s %s %s", result, reqErr.StatusCode(), reqErr.RequestID())
		}
	} else {
		result = err.Error()
	}
	return result
}

// Verify that the ECS cluster exists.
func VerifyClusterExists(ecs_obj *ecs.ECS, cluster string) error {
	params := &ecs.DescribeClustersInput{
		Clusters: []*string{
			aws.String(cluster),
		},
	}
	clusters, err := ecs_obj.DescribeClusters(params)

	if err != nil {
		return fmt.Errorf("Cannot verify if ECS cluster exists: %s", FormatAwsError(err))
	}
	if len(clusters.Clusters) == 0 {
		return fmt.Errorf("Error: ECS Cluster '%s' does not exist, cannot proceed.\n", cluster)
	}
	if len(clusters.Clusters) != 1 {
		return fmt.Errorf("Error: Unexpected number of ECS Clusters returned when searching for '%s'. Received: %+v\n", cluster, clusters.Clusters)
	}
	return nil
}

// Verify that the ECS service exists.
func VerifyServiceExists(ecs_obj *ecs.ECS, cluster string, service string) error {
	params := &ecs.DescribeServicesInput{
		Cluster: &cluster,
		Services: []*string{ // Required
			aws.String(service), // Required
		},
	}
	_, err := ecs_obj.DescribeServices(params)

	if err != nil {
		return fmt.Errorf("Cannot verify if ECS service exists: %s", FormatAwsError(err))
	}
	return nil
}

func GetContainerInstanceArnsForService(ecs_obj *ecs.ECS, cluster string, service string, local_container_instance_arn string, debug bool) ([]string, error) {
	// Fetch a task list based on the service name.
	list_tasks_params := &ecs.ListTasksInput{
		Cluster:     &cluster,
		ServiceName: &service,
	}
	list_tasks_resp, list_tasks_err := ecs_obj.ListTasks(list_tasks_params)

	if list_tasks_err != nil {
		return []string{}, fmt.Errorf("Cannot retrieve ECS task list: %s", FormatAwsError(list_tasks_err))
	}

	if len(list_tasks_resp.TaskArns) <= 0 {
		return []string{}, fmt.Errorf("No ECS tasks found with specified filter - cluster: ", cluster, ", service:", service)
	}

	// Describe the tasks retrieved above.
	describe_tasks_params := &ecs.DescribeTasksInput{
		Cluster: &cluster,
		Tasks:   list_tasks_resp.TaskArns,
	}
	describe_tasks_resp, describe_tasks_err := ecs_obj.DescribeTasks(describe_tasks_params)

	if describe_tasks_err != nil {
		return []string{}, fmt.Errorf("Cannot retrieve ECS task details:", FormatAwsError(describe_tasks_err))
	}

	if len(describe_tasks_resp.Tasks) <= 0 {
		return []string{}, fmt.Errorf("No ECS task details found with specified filter - tasks:", strings.Join(aws.StringValueSlice(list_tasks_resp.TaskArns), ", "))
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
		return []string{}, fmt.Errorf("No ECS task results found in RUNNING state, no ECS container instances to return.")
	}
	return result, nil
}

func GetEc2InstanceIdsFromContainerInstances(ecs_obj *ecs.ECS, cluster string, container_instances []string, debug bool) ([]string, error) {
	params := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(cluster),
		ContainerInstances: aws.StringSlice(container_instances),
	}
	resp, err := ecs_obj.DescribeContainerInstances(params)

	if err != nil {
		return []string{}, fmt.Errorf("Cannot retrieve ECS container instance information: %s", FormatAwsError(err))
	}

	if len(resp.ContainerInstances) <= 0 {
		return []string{}, fmt.Errorf("No ECS container instances found with specified filter - cluster:", cluster, "- instances:", strings.Join(container_instances, ", "))
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
		return []string{}, fmt.Errorf("No running ECS container instances found in result set, cannot proceed.")
	}
	return result, nil
}

func GetEc2PrivateIpsFromInstanceIds(ec2_obj *ec2.EC2, instance_ids []string, debug bool) ([]string, error) {
	params := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(instance_ids),
	}
	resp, err := ec2_obj.DescribeInstances(params)

	if err != nil {
		return []string{}, fmt.Errorf("Cannot retrieve EC2 instance information: %s", FormatAwsError(err))
	}

	if len(resp.Reservations) <= 0 {
		return []string{}, fmt.Errorf("No EC2 instances found (Reservations.*) with specified Instance IDs filter: ", strings.Join(instance_ids, ", "))
	}
	if len(resp.Reservations[0].Instances) <= 0 {
		return []string{}, fmt.Errorf("No EC2 instances found (Reservations[0].* with specified Instance IDs filter: ", strings.Join(instance_ids, ", "))
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
		return []string{}, fmt.Errorf("No running EC2 instances found in result set, cannot proceed.")
	}
	return result, nil
}
