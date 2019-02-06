# kops-autoscaler-openstack

The purpose of this application is to provide capability to scale cluster up/down in case of need. This application supports currently two use-cases:

- if kops instancegroup minsize is larger than current instances in openstack -> scale up 
- if kops instancegroup maxsize is smaller than current instances in openstack -> scale down

This application will detect the need of change by running `kops update cluster <cluster>`. Scaling means that this application will execute `kops update cluster <cluster> --yes` under the hood.

This application makes it possible to use `kops rolling-update <cluster>` command in openstack kops. 

### How to install

See Examples

### How to contribute

Make issues/PRs