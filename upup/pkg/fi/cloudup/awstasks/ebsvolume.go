package awstasks

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
)

//go:generate fitask -type=EBSVolume
type EBSVolume struct {
	Name             *string
	ID               *string
	AvailabilityZone *string
	VolumeType       *string
	SizeGB           *int64
	Tags             map[string]string
}

var _ fi.CompareWithID = &EBSVolume{}

func (e *EBSVolume) CompareWithID() *string {
	return e.ID
}

type TaggableResource interface {
	FindResourceID(c fi.Cloud) (*string, error)
}

var _ TaggableResource = &EBSVolume{}

func (e *EBSVolume) FindResourceID(c fi.Cloud) (*string, error) {
	actual, err := e.find(c.(*awsup.AWSCloud))
	if err != nil {
		return nil, fmt.Errorf("error querying for EBSVolume: %v", err)
	}
	if actual == nil {
		return nil, nil
	}
	return actual.ID, nil
}

func (e *EBSVolume) Find(context *fi.Context) (*EBSVolume, error) {
	actual, err := e.find(context.Cloud.(*awsup.AWSCloud))
	if actual != nil && err == nil {
		e.ID = actual.ID
	}
	return actual, err
}

func (e *EBSVolume) find(cloud *awsup.AWSCloud) (*EBSVolume, error) {
	filters := cloud.BuildFilters(e.Name)
	request := &ec2.DescribeVolumesInput{
		Filters: filters,
	}

	response, err := cloud.EC2.DescribeVolumes(request)
	if err != nil {
		return nil, fmt.Errorf("error listing volumes: %v", err)
	}

	if response == nil || len(response.Volumes) == 0 {
		return nil, nil
	}

	if len(response.Volumes) != 1 {
		return nil, fmt.Errorf("found multiple Volumes with name: %s", *e.Name)
	}
	glog.V(2).Info("found existing volume")
	v := response.Volumes[0]
	actual := &EBSVolume{
		ID:               v.VolumeId,
		AvailabilityZone: v.AvailabilityZone,
		VolumeType:       v.VolumeType,
		SizeGB:           v.Size,
		Name:             e.Name,
	}

	actual.Tags = mapEC2TagsToMap(v.Tags)

	return actual, nil
}

func (e *EBSVolume) Run(c *fi.Context) error {
	c.Cloud.(*awsup.AWSCloud).AddTags(e.Name, e.Tags)
	return fi.DefaultDeltaRunMethod(e, c)
}

func (_ *EBSVolume) CheckChanges(a, e, changes *EBSVolume) error {
	if a == nil {
		if e.Name == nil {
			return fi.RequiredField("Name")
		}
	}
	if a != nil {
		if changes.ID != nil {
			return fi.CannotChangeField("ID")
		}
	}
	return nil
}

func (_ *EBSVolume) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *EBSVolume) error {
	if a == nil {
		glog.V(2).Infof("Creating PersistentVolume with Name:%q", *e.Name)

		request := &ec2.CreateVolumeInput{
			Size:             e.SizeGB,
			AvailabilityZone: e.AvailabilityZone,
			VolumeType:       e.VolumeType,
		}

		response, err := t.Cloud.EC2.CreateVolume(request)
		if err != nil {
			return fmt.Errorf("error creating PersistentVolume: %v", err)
		}

		e.ID = response.VolumeId
	}

	return t.AddAWSTags(*e.ID, e.Tags)
}

type terraformVolume struct {
	AvailabilityZone *string           `json:"availability_zone"`
	Size             *int64            `json:"size"`
	Type             *string           `json:"type"`
	Tags             map[string]string `json:"tags,omitempty"`
}

func (_ *EBSVolume) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *EBSVolume) error {
	tf := &terraformVolume{
		AvailabilityZone: e.AvailabilityZone,
		Size:             e.SizeGB,
		Type:             e.VolumeType,
		Tags:             e.Tags,
	}

	return t.RenderResource("aws_ebs_volume", *e.Name, tf)
}

func (e *EBSVolume) TerraformLink() *terraform.Literal {
	return terraform.LiteralSelfLink("aws_ebs_volume", *e.Name)
}
