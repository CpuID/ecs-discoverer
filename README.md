# Amazon ECS Service Discovery Tool

[![Build Status](https://travis-ci.org/CpuID/ecs-discoverer.svg?branch=master)](https://travis-ci.org/CpuID/ecs-discoverer)

Designed as a Cluster Orchestration helper, more info below.

## Disclaimer

This is one of my first Golang projects I have completed start-to-finish, the codebase could use
some splitting up/separation into more modular pieces, but it performs the task required for now
so not really too concerned. I spent less than a day in total on it.
I am sure there are lots of improvements that can be made, but being it runs
and exits in under 300ms (most of that AWS API latencies), I am happy :)

## Details

So let's say you want to orchestrate a cluster of containers running on top of Amazon ECS,
for example a service like Consul. You need to obtain the IP addresses of all other potential
members/nodes to be able to attempt to join one. This utility provides you the other node addresses to use.

This could easily be achieved with awscli + bash + jq, but it is a handful of API calls,
and sifting through results so I opted to do it all in a single binary instead.

## Prerequisites

Designed to be run on EC2 within an Amazon ECS cluster, inside a Docker container (and with the default networking topology).
This utility will attempt to access the ECS agent on http://172.17.0.1:51678/ in addition to the AWS APIs. Access to the local
Docker daemon socket is not required. In future ideally this would support handling Docker networks, with a custom IP to hit,
for now it just hits the default bridge network docker0 IP.

## Service/Task Ports

There is an expectation you will know the service port already, and all tasks under a given service
will have that port open. This utility only deals with retrieving the correct IP addresses, ports
are not covered at all.

## Example

I have 2 ECS container instances in a single cluster, with 2 services running.
One of the services is named "nginx", and has a desired count of 2 (one on each of the ECS
container instances for now). The result will be a CSV of VPC/private IPs, excluding the current
instance (you normally don't want to attempt to join yourself if orchestrating a cluster):

```
root@cddb6164b344:/# ./ecs-discoverer -s nginx
10.20.0.97
```

Use `./ecs-discoverer --help` for a full help listing.

## Building

```
go build
```

Single binary, copy/put into the Docker container of your choice. Or download a binary release if it suffices for you.

## IAM Policy

Make sure your ECS container instances have a policy containing the below (feel free to lock down the Resource by account/region):

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "ecs:DescribeContainerInstances",
                "ecs:DescribeServices",
                "ecs:DescribeTasks",
                "ecs:ListTasks"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

## License

Released under MIT License.
