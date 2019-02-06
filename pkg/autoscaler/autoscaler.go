package autoscaler

import (
	"fmt"
	//"strings"
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/util/pkg/vfs"
)

// Options contains startup variables from cobra cmd
type Options struct {
	Sleep          int
	StateStore     string
	AccessKey      string
	SecretKey      string
	CustomEndpoint string
	ClusterName    string
}

type openstackASG struct {
	ApplyCmd  *cloudup.ApplyClusterCmd
	clientset simple.Clientset
	opts      *Options
}

// Run will execute cluster check in loop periodically
func Run(opts *Options) error {
	registryBase, err := vfs.Context.BuildVfsPath(opts.StateStore)
	if err != nil {
		return fmt.Errorf("error parsing registry path %q: %v", opts.StateStore, err)
	}

	clientset := vfsclientset.NewVFSClientset(registryBase, true)
	osASG := &openstackASG{
		opts:      opts,
		clientset: clientset,
	}
	for {
		time.Sleep(time.Duration(opts.Sleep) * time.Second)
		glog.Infof("Executing...\n")

		err := osASG.updateApplyCmd()
		if err != nil {
			glog.Errorf("Error updating applycmd %v", err)
			continue
		}

		needsUpdate, err := osASG.dryRun()
		if err != nil {
			glog.Errorf("Error running dryrun %v", err)
			continue
		}

		if needsUpdate {
			err = osASG.update()
			if err != nil {
				glog.Errorf("Error updating cluster %v", err)
			}
		}
	}
	return nil
}

func (osASG *openstackASG) updateApplyCmd() error {
	cluster, err := osASG.clientset.GetCluster(osASG.opts.ClusterName)
	if err != nil {
		return fmt.Errorf("error initializing cluster %v", err)
	}

	list, err := osASG.clientset.InstanceGroupsFor(cluster).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	var instanceGroups []*kops.InstanceGroup
	for i := range list.Items {
		instanceGroups = append(instanceGroups, &list.Items[i])
	}

	osASG.ApplyCmd = &cloudup.ApplyClusterCmd{
		Clientset:      osASG.clientset,
		Cluster:        cluster,
		InstanceGroups: instanceGroups,
		Phase:          cloudup.PhaseCluster,
		TargetName:     cloudup.TargetDryRun,
		OutDir:         "out",
		Models:         []string{"proto", "cloudup"},
	}
	return nil
}

func (osASG *openstackASG) dryRun() (bool, error) {
	osASG.ApplyCmd.TargetName = cloudup.TargetDryRun
	osASG.ApplyCmd.DryRun = true

	if err := osASG.ApplyCmd.Run(); err != nil {
		return false, err
	}
	target := osASG.ApplyCmd.Target.(*fi.DryRunTarget)
	if target.HasChanges() {
		// This does not work yet, waiting for PR to be approved
		/*for _, r := range target.Changes() {
			if strings.HasPrefix(r, "Instance") {
				glog.Infof("Found instance in tasks running update --yes\n")
				return true, nil
			}
		}*/
	}
	return false, nil
}

func (osASG *openstackASG) update() error {
	osASG.ApplyCmd.TargetName = cloudup.TargetDirect
	osASG.ApplyCmd.DryRun = false
	var options fi.RunTasksOptions
	options.InitDefaults()
	osASG.ApplyCmd.RunTasksOptions = &options
	if err := osASG.ApplyCmd.Run(); err != nil {
		return err
	}
	return nil
}
