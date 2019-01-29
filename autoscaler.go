package main

import (
	"flag"
	"os"
	"fmt"
	"time"
	"strings"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/util/pkg/vfs"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
)

type OpenstackASG struct {
	ApplyCmd    *cloudup.ApplyClusterCmd
	clientset   simple.Clientset
	clusterName string
}

var flagRegistryBase = flag.String("registry", os.Getenv("KOPS_STATE_STORE"), "VFS path where files are kept")
var flagClusterName = flag.String("name", os.Getenv("NAME"), "Name of cluster")

func main() {
	asg := OpenstackASG{}
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")
	glog.Infof("Starting application...\n")
	glog.Flush()
	err := asg.parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	asg.loopUntil()
}

func (a *OpenstackASG) parseFlags() error {
	registryBase, err := vfs.Context.BuildVfsPath(*flagRegistryBase)
	if err != nil {
		return fmt.Errorf("error parsing registry path %q: %v", *flagRegistryBase, err)
	}

	clusterName := *flagClusterName
	if clusterName == "" {
		return fmt.Errorf("Must pass NAME environment variable")
	}

	clientset := vfsclientset.NewVFSClientset(registryBase, true)

	a.clientset = clientset
	a.clusterName = clusterName
	err = a.updateApplyCmd()
	if err != nil {
		return err
	}
	return nil
}

func (a *OpenstackASG) updateApplyCmd() error {
	cluster, err := a.clientset.GetCluster(a.clusterName)
	if err != nil {
		return fmt.Errorf("error initializing cluster %v", err)
	}

	list, err := a.clientset.InstanceGroupsFor(cluster).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	var instanceGroups []*kops.InstanceGroup
	for i := range list.Items {
		instanceGroups = append(instanceGroups, &list.Items[i])
	}

	a.ApplyCmd = &cloudup.ApplyClusterCmd{
		Clientset:      a.clientset,
		Cluster:        cluster,
		InstanceGroups: instanceGroups,
		Phase:          cloudup.PhaseCluster,
		TargetName:     cloudup.TargetDryRun,
		OutDir:         "out",
		Models:         []string{"proto", "cloudup"},
	}
	return nil
}

func (a *OpenstackASG) loopUntil() {
	for {
		// TODO make this configurable
		time.Sleep(60 * time.Second)
		glog.Infof("Executing...\n")
		err := a.updateApplyCmd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		update, err := a.dryRun()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		if update {
			err = a.update()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				continue
			}
		}
	}
}

func getTaskName(t fi.Task) string {
	s := fmt.Sprintf("%T", t)
	lastDot := strings.LastIndexByte(s, '.')
	if lastDot != -1 {
		s = s[lastDot+1:]
	}
	return s
}

func (a *OpenstackASG) dryRun() (bool, error) {
	a.ApplyCmd.TargetName = cloudup.TargetDryRun
	a.ApplyCmd.DryRun = true
	needsCreate := false

	if err := a.ApplyCmd.Run(); err != nil {
		return needsCreate, err
	}
	target := a.ApplyCmd.Target.(*fi.DryRunTarget)
	if target.HasChanges() {
		for _, r := range target.Changes() {
			if strings.HasPrefix(getTaskName(r), "Instance") {
				glog.Infof("Found instance in tasks running update --yes\n")
				return true, nil
			}
		}
	}
	return needsCreate, nil
}

func (a *OpenstackASG) update() error {
	a.ApplyCmd.TargetName = cloudup.TargetDirect
	a.ApplyCmd.DryRun = false
	var options fi.RunTasksOptions
	options.InitDefaults()
	a.ApplyCmd.RunTasksOptions = &options
	if err := a.ApplyCmd.Run(); err != nil {
		return err
	}
	return nil
}
