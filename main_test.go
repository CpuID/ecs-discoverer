package main

import (
	"flag"
	"github.com/codegangsta/cli"
	"testing"
)

func TestParseFlags(t *testing.T) {
	set1 := flag.NewFlagSet("test1", 0)
	set1.String("c", "somecluster", "doc")
	set1.String("r", "someregion", "doc")
	set1.String("s", "someservice", "doc")
	context1 := cli.NewContext(nil, set1, nil)
	current_cluster1, aws_region1, cluster1, service1, debug1 := parseFlags(context1)
	if current_cluster1 != false {
		t.Error("Expected current_cluster1 to be false, got", current_cluster1)
	}
	if cluster1 != "somecluster" {
		t.Error("Expected cluster1 to be somecluster, got", cluster1)
	}
	if aws_region1 != "someregion" {
		t.Error("Expected aws_region1 to be someregion, got", aws_region1)
	}
	if service1 != "someservice" {
		t.Error("Expected service1 to be someservice, got", service1)
	}
	if debug1 != false {
		t.Error("Expected debug1 to be false, got", debug1)
	}
	//
	set2 := flag.NewFlagSet("test2", 0)
	set2.String("s", "someotherservice", "doc")
	set2.Bool("d", true, "doc")
	context2 := cli.NewContext(nil, set2, nil)
	current_cluster2, aws_region2, cluster2, service2, debug2 := parseFlags(context2)
	if current_cluster2 != true {
		t.Error("Expected current_cluster1 to be true, got", current_cluster1)
	}
	if cluster2 != "" {
		t.Error("Expected cluster2 to be empty '', got", cluster2)
	}
	if aws_region2 != "" {
		t.Error("Expected aws_region2 to be empty '', got", aws_region2)
	}
	if service2 != "someotherservice" {
		t.Error("Expected service2 to be someservice, got", service2)
	}
	if debug2 != true {
		t.Error("Expected debug2 to be true, got", debug2)
	}
}
