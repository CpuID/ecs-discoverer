// Written by Nathan Sullivan <nathan@nightsys.net>

// Consul discoverer binary that searches for all other consul servers and returns the IP addresses
// to allow a new Consul process to join the existing cluster.

// Designed to be run on EC2 within an ECS cluster, inside a Docker container (and with the default networking topology).

// Would have required lots of hacks and magic to do this in bash + awscli + jq, easier this way.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type EcsAgentMetadata struct {
	Cluster              string `json:"Cluster"`
	ContainerInstanceArn string `json:"ContainerInstanceArn"`
	Version              string `json:"Version"`
}

func getEcsAgentMetadata() EcsAgentMetadata {
	resp, err := http.Get("http://172.17.42.1:51678/v1/metadata")
	if err != nil {
		fmt.Println("Error retrieving metadata from ECS agent on local Docker host (via 172.17.42.1:51678):")
		fmt.Println(err)
		os.Exit(1)
	}

	defer resp.Body.Close()
	body, read_err := ioutil.ReadAll(resp.Body)
	if read_err != nil {
		fmt.Println("Error reading the metadata response from ECS agent on local Docker host:")
		fmt.Println(read_err)
		os.Exit(1)
	}

	var json_data EcsAgentMetadata
	json_data_err := json.Unmarshal(body, &json_data)
	if json_data_err != nil {
		fmt.Println("Error parsing JSON response from ECS agent on local Docker host:")
		fmt.Println(json_data_err)
		os.Exit(1)
	}

	return json_data
}

// Save some repetition, formatting the output of these.
func formatAwsError(err error) {
	if awsErr, ok := err.(awserr.Error); ok {
		// Generic AWS Error with Code, Message, and original error (if any)
		fmt.Println(awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			// A service error occurred
			fmt.Println(reqErr.StatusCode(), reqErr.RequestID())
		}
	} else {
		fmt.Println(err.Error())
	}
}

// Verify that the ECS service exists.
func verifyServiceExists(ecs_obj *ecs.ECS, cluster string, service string) {
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

func getContainerInstanceArnsForService(ecs_obj *ecs.ECS, cluster string, service string, local_container_instance_arn string) []string {
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
		fmt.Println("No ECS tasks found with specified filter - cluster: ", cluster, ", service: ", service)
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
		fmt.Println("No ECS task details found with specified filter - tasks: ", strings.Join(aws.StringValueSlice(list_tasks_resp.TaskArns), ", "))
		os.Exit(1)
	}

	var result []string
	for _, value := range describe_tasks_resp.Tasks {
		if *value.LastStatus == "RUNNING" && *value.ContainerInstanceArn != local_container_instance_arn {
			result = append(result, *value.ContainerInstanceArn)
		}
		// TODOLATER - debug output to explain why task was not included in results.
	}

	if len(result) == 0 {
		fmt.Println("No ECS task results found in RUNNING state, no ECS container instances to return.")
		os.Exit(1)
	}
	return result
}

func getEc2InstanceIdsFromContainerInstances(ecs_obj *ecs.ECS, cluster string, container_instances []string) []string {
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
		fmt.Println("No ECS container instances found with specified filter - cluster: ", cluster, " - instances: ", strings.Join(container_instances, ", "))
		os.Exit(1)
	}

	var result []string
	for _, value := range resp.ContainerInstances {
		if *value.Status == "ACTIVE" {
			result = append(result, *value.Ec2InstanceId)
		}
		// TODOLATER - debug output that container instance was not active in else clause.
	}

	if len(result) == 0 {
		fmt.Println("No running ECS container instances found in result set, cannot proceed.")
		os.Exit(1)
	}
	return result
}

func getEc2PrivateIpsFromInstanceIds(ec2_obj *ec2.EC2, instance_ids []string) []string {
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
	for _, value := range resp.Reservations[0].Instances {
		if *value.State.Name == "running" {
			result = append(result, *value.PrivateIpAddress)
		}
		// TODOLATER - debug output that instance was not running in else clause.
	}

	if len(result) == 0 {
		fmt.Println("No running EC2 instances found in result set, cannot proceed.")
		os.Exit(1)
	}
	return result
}

func main() {
	// Check that the ECS service name is passed in as an argument.
	if len(os.Args) != 2 {
		fmt.Println("Usage: ", os.Args[0], " ecs_service_name")
		os.Exit(1)
	}
	ecs_service := os.Args[1]

	// Get the metadata from the ECS agent on the local Docker host.
	local_ecs_agent_metadata := getEcsAgentMetadata()

	// Discover the region which this instance resides.
	metadata := ec2metadata.New(&ec2metadata.Config{})
	region, err := metadata.Region()
	if err != nil {
		fmt.Println("Cannot retrieve AWS region from EC2 Metadata Service:")
		formatAwsError(err)
		os.Exit(1)
	}

	// Discover the ECS cluster this EC2 instance belongs to, via local ECS agent.
	ecs_cluster := local_ecs_agent_metadata.Cluster

	// Reusable config object for AWS services with current region attached.
	aws_config := &aws.Config{Region: aws.String(region)}

	// Create an ECS service object.
	ecs_obj := ecs.New(aws_config)
	// Create an EC2 service object.
	ec2_obj := ec2.New(aws_config)

	// Check that the service exists.
	verifyServiceExists(ecs_obj, ecs_cluster, ecs_service)

	// TODO - do we want to get the listen ports for each task? or just assume a port...?

	// Get all tasks for the given service, in this ECS cluster. We exclude the current container instance in the result,
	// as we only need to know about all other instances.
	container_instances := getContainerInstanceArnsForService(ecs_obj, ecs_cluster, ecs_service, local_ecs_agent_metadata.ContainerInstanceArn)

	// Get EC2 instance IDs for all container instances returned.
	instance_ids := getEc2InstanceIdsFromContainerInstances(ecs_obj, ecs_cluster, container_instances)

	// Get the private IP of the EC2 (container) instance running the ECS agent.
	instance_private_ips := getEc2PrivateIpsFromInstanceIds(ec2_obj, instance_ids)

	// TODO - return the IPs in an acceptable format for use as consul join line?
	fmt.Println(strings.Join(instance_private_ips, ","))
}
