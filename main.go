package main

import (
	"flag"
	"os"
	"fmt"
	"time"

	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/util/pkg/vfs"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstack"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstacktasks"
)

type OpenstackASG struct {
	RegistryBase vfs.Path
	ConfigBase   vfs.Path
	ClusterName  string
	Cluster      *kops.Cluster
	Clientset    simple.Clientset
}


var flagRegistryBase = flag.String("registry", os.Getenv("KOPS_STATE_STORE"), "VFS path where files are kept")
var flagClusterName = flag.String("name", os.Getenv("NAME"), "Name of cluster")

func main() {
	asg := OpenstackASG{}
	flag.Parse()
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

	configBase, err := vfs.Context.BuildVfsPath(*flagRegistryBase + "/" + *flagClusterName)
	if err != nil {
		return fmt.Errorf("error parsing config path %q: %v", configBase, err)
	}

	clientset := vfsclientset.NewVFSClientset(registryBase, true)
	cluster, err := clientset.GetCluster(clusterName)
	if err != nil {
		return fmt.Errorf("error initializing cluster %v", err)
	}

	a.RegistryBase = registryBase
	a.ClusterName = clusterName
	a.Clientset = clientset
	a.Cluster = cluster
	a.ConfigBase = configBase

	return nil
}

// the idea of this function is that it will loop forever
// and compare KOPS_STATE_STORE ig state towards what we have in cloud
// if count does not match, it will call update
func (a *OpenstackASG) loopUntil() {
	for {
		time.Sleep(10 * time.Second)
		err := a.listInstanceGroups()
		if err != nil {
			// TODO better logger
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}

	}

}

func (a *OpenstackASG) listInstanceGroups() error {

	isDryrun := true

	l := &Loader{}
	l.Init()
	l.Cluster = a.Cluster

	l.AddTypes(map[string]interface{}{
		"instance": &openstacktasks.Instance{},
	})

	keyStore, err := a.Clientset.KeyStore(a.Cluster)
	if err != nil {
		return err
	}

	secretStore, err := a.Clientset.SecretStore(a.Cluster)
	if err != nil {
		return err
	}
	assetBuilder := assets.NewAssetBuilder(a.Cluster, "cluster")
	target := fi.NewDryRunTarget(assetBuilder, os.Stdout)

	modelStore, err := cloudup.findModelStore()
	if err != nil {
		return err
	}

	osc, err := openstack.NewOpenstackCloud(cloudTags, &a.Cluster.Spec)
	if err != nil {
		return nil, err
	}

	var fileModels []string
	stageAssetsLifecycle := fi.LifecycleIgnore
	var lifecycleOverrides map[string]fi.Lifecycle

	taskMap, err := l.BuildTasks(modelStore, fileModels, assetBuilder, &stageAssetsLifecycle, lifecycleOverrides)
	if err != nil {
		return fmt.Errorf("error building tasks: %v", err)
	}
	fmt.Printf("%+v", taskMap)

	context, err := fi.NewContext(target, a.Cluster, osc, keyStore, secretStore, a.ConfigBase, true, taskMap)
	if err != nil {
		return fmt.Errorf("error building context: %v", err)
	}
	defer context.Close()

	var options fi.RunTasksOptions
	options.InitDefaults()

	err = context.RunTasks(options)
	if err != nil {
		return fmt.Errorf("error running tasks: %v", err)
	}

	err = target.Finish(taskMap) //This will finish the apply, and print the changes
	if err != nil {
		return fmt.Errorf("error closing target: %v", err)
	}

}