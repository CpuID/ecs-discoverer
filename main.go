// Written by Nathan Sullivan <nathan@nightsys.net>

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/CpuID/ecs-discoverer/shared"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/codegangsta/cli"
)

type EcsAgentMetadata struct {
	Cluster              string `json:"Cluster"`
	ContainerInstanceArn string `json:"ContainerInstanceArn"`
	Version              string `json:"Version"`
}

// TODO: deal with docker custom networks. for now this just hits the default bridge network IP.
func getEcsAgentMetadata() EcsAgentMetadata {
	resp, err := http.Get("http://172.17.0.1:51678/v1/metadata")
	if err != nil {
		fmt.Println("Error retrieving metadata from ECS agent on local Docker host (via 172.17.0.1:51678):")
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

func doMain(current_cluster bool, aws_region string, cluster_name string, ecs_service string, debug bool) {
	var local_ecs_agent_metadata EcsAgentMetadata
	if current_cluster == true {
		// Get the metadata from the ECS agent on the local Docker host.
		local_ecs_agent_metadata = getEcsAgentMetadata()
	}

	var region string
	if aws_region != "" {
		region = aws_region
	} else {
		// Discover the region which this instance resides.
		metadata := ec2metadata.New(session.New())
		use_region, err := metadata.Region()
		if err != nil {
			fmt.Println("Cannot retrieve AWS region from EC2 Metadata Service:")
			shared.FormatAwsError(err)
			os.Exit(1)
		}
		region = use_region
	}

	// Reusable config session object for AWS services with current region attached.
	aws_config_session := session.New(&aws.Config{Region: aws.String(region)})

	// Create an ECS service object.
	ecs_obj := ecs.New(aws_config_session)
	// Create an EC2 service object.
	ec2_obj := ec2.New(aws_config_session)

	var ecs_cluster string
	if current_cluster == true {
		// Discover the ECS cluster this EC2 instance belongs to, via local ECS agent.
		ecs_cluster = local_ecs_agent_metadata.Cluster
	} else {
		// Use the user provided WAN cluster, for connecting to services in a different ECS Cluster.
		// First we verify the cluster exists before proceeding.
		shared.VerifyClusterExists(ecs_obj, cluster_name)
		ecs_cluster = cluster_name
	}

	// Check that the service exists.
	shared.VerifyServiceExists(ecs_obj, ecs_cluster, ecs_service)

	// TODOLATER - do we want to get the listen ports for each task? or just assume a port...?
	// readme states ports are outside the scope for now.

	// Get all tasks for the given service, in this ECS cluster. We exclude the current container instance in the result,
	// as we only need to know about all other instances. The exclusion only occurs when we are working on the current cluster.
	current_container_instance_arn := "NONE"
	if current_cluster == true {
		current_container_instance_arn = local_ecs_agent_metadata.ContainerInstanceArn
	}
	container_instances := shared.GetContainerInstanceArnsForService(ecs_obj, ecs_cluster, ecs_service, current_container_instance_arn, debug)
	if debug == true {
		fmt.Println("container_instances:", strings.Join(container_instances, ","))
	}

	// Get EC2 instance IDs for all container instances returned.
	instance_ids := shared.GetEc2InstanceIdsFromContainerInstances(ecs_obj, ecs_cluster, container_instances, debug)
	if debug == true {
		fmt.Println("instance_ids:", strings.Join(instance_ids, ","))
	}

	// Get the private IP of the EC2 (container) instance running the ECS agent.
	instance_private_ips := shared.GetEc2PrivateIpsFromInstanceIds(ec2_obj, instance_ids, debug)
	if debug == true {
		fmt.Println("instance_private_ips:", strings.Join(instance_private_ips, ","))
	}

	fmt.Println(strings.Join(instance_private_ips, ","))
}

// current_cluster, aws_region, cluster, service, debug
func parseFlags(c *cli.Context) (bool, string, string, string, bool) {
	current_cluster := false
	cluster := ""
	aws_region := ""
	if c.String("c") == "" {
		current_cluster = true
	} else {
		cluster = c.String("c")
		if c.String("r") == "" {
			fmt.Printf("Error: If Cluster (-c) is specified, AWS Region (-r) is also required. Cannot proceed.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}
		aws_region = c.String("r")
	}
	if c.String("s") == "" {
		fmt.Printf("Error: Service (-s) must not be empty. Cannot proceed.\n\n")
		cli.ShowAppHelp(c)
		os.Exit(1)
	}
	return current_cluster, aws_region, cluster, c.String("s"), c.Bool("d")
}

func main() {
	app := cli.NewApp()
	app.Name = "ecs-discoverer"
	app.Version = ecs_discoverer_version
	app.Usage = "Discovery tool for Private IPs of ECS EC2 Container Instances for a given Service/Cluster"
	app.Action = func(c *cli.Context) {
		current_cluster, aws_region, cluster, service, debug := parseFlags(c)
		doMain(current_cluster, aws_region, cluster, service, debug)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "c",
			Value: "",
			Usage: "ECS Cluster Name (Optional - defaults to the ECS Cluster of this instance only)",
		},
		cli.BoolFlag{
			Name:  "d",
			Usage: "Debug Mode",
		},
		cli.StringFlag{
			Name:  "r",
			Value: "",
			Usage: "AWS Region (Optional - defaults to the location of this ECS Cluster instance)",
		},
		cli.StringFlag{
			Name:  "s",
			Value: "",
			Usage: "ECS Service Name (Required)",
		},
	}

	app.Run(os.Args)
}
