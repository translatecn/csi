/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1 "local/3rd/external-snapshotter/client/apis/volumesnapshot/v1"
)

// FakeVolumeSnapshotContents implements VolumeSnapshotContentInterface
type FakeVolumeSnapshotContents struct {
	Fake *FakeSnapshotV1
}

var volumesnapshotcontentsResource = v1.SchemeGroupVersion.WithResource("volumesnapshotcontents")

var volumesnapshotcontentsKind = v1.SchemeGroupVersion.WithKind("VolumeSnapshotContent")

// Get takes name of the volumeSnapshotContent, and returns the corresponding volumeSnapshotContent object, and an error if there is any.
func (c *FakeVolumeSnapshotContents) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.VolumeSnapshotContent, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(volumesnapshotcontentsResource, name), &v1.VolumeSnapshotContent{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.VolumeSnapshotContent), err
}

// List takes label and field selectors, and returns the list of VolumeSnapshotContents that match those selectors.
func (c *FakeVolumeSnapshotContents) List(ctx context.Context, opts metav1.ListOptions) (result *v1.VolumeSnapshotContentList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(volumesnapshotcontentsResource, volumesnapshotcontentsKind, opts), &v1.VolumeSnapshotContentList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.VolumeSnapshotContentList{ListMeta: obj.(*v1.VolumeSnapshotContentList).ListMeta}
	for _, item := range obj.(*v1.VolumeSnapshotContentList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested volumeSnapshotContents.
func (c *FakeVolumeSnapshotContents) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(volumesnapshotcontentsResource, opts))
}

// Create takes the representation of a volumeSnapshotContent and creates it.  Returns the server's representation of the volumeSnapshotContent, and an error, if there is any.
func (c *FakeVolumeSnapshotContents) Create(ctx context.Context, volumeSnapshotContent *v1.VolumeSnapshotContent, opts metav1.CreateOptions) (result *v1.VolumeSnapshotContent, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(volumesnapshotcontentsResource, volumeSnapshotContent), &v1.VolumeSnapshotContent{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.VolumeSnapshotContent), err
}

// Update takes the representation of a volumeSnapshotContent and updates it. Returns the server's representation of the volumeSnapshotContent, and an error, if there is any.
func (c *FakeVolumeSnapshotContents) Update(ctx context.Context, volumeSnapshotContent *v1.VolumeSnapshotContent, opts metav1.UpdateOptions) (result *v1.VolumeSnapshotContent, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(volumesnapshotcontentsResource, volumeSnapshotContent), &v1.VolumeSnapshotContent{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.VolumeSnapshotContent), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVolumeSnapshotContents) UpdateStatus(ctx context.Context, volumeSnapshotContent *v1.VolumeSnapshotContent, opts metav1.UpdateOptions) (*v1.VolumeSnapshotContent, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(volumesnapshotcontentsResource, "status", volumeSnapshotContent), &v1.VolumeSnapshotContent{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.VolumeSnapshotContent), err
}

// Delete takes name of the volumeSnapshotContent and deletes it. Returns an error if one occurs.
func (c *FakeVolumeSnapshotContents) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(volumesnapshotcontentsResource, name, opts), &v1.VolumeSnapshotContent{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVolumeSnapshotContents) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(volumesnapshotcontentsResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1.VolumeSnapshotContentList{})
	return err
}

// Patch applies the patch and returns the patched volumeSnapshotContent.
func (c *FakeVolumeSnapshotContents) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.VolumeSnapshotContent, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(volumesnapshotcontentsResource, name, pt, data, subresources...), &v1.VolumeSnapshotContent{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.VolumeSnapshotContent), err
}
