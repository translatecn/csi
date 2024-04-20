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

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1alpha1 "local/3rd/external-snapshotter/client/apis/volumegroupsnapshot/v1alpha1"
)

// FakeVolumeGroupSnapshotClasses implements VolumeGroupSnapshotClassInterface
type FakeVolumeGroupSnapshotClasses struct {
	Fake *FakeGroupsnapshotV1alpha1
}

var volumegroupsnapshotclassesResource = v1alpha1.SchemeGroupVersion.WithResource("volumegroupsnapshotclasses")

var volumegroupsnapshotclassesKind = v1alpha1.SchemeGroupVersion.WithKind("VolumeGroupSnapshotClass")

// Get takes name of the volumeGroupSnapshotClass, and returns the corresponding volumeGroupSnapshotClass object, and an error if there is any.
func (c *FakeVolumeGroupSnapshotClasses) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.VolumeGroupSnapshotClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(volumegroupsnapshotclassesResource, name), &v1alpha1.VolumeGroupSnapshotClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VolumeGroupSnapshotClass), err
}

// List takes label and field selectors, and returns the list of VolumeGroupSnapshotClasses that match those selectors.
func (c *FakeVolumeGroupSnapshotClasses) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.VolumeGroupSnapshotClassList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(volumegroupsnapshotclassesResource, volumegroupsnapshotclassesKind, opts), &v1alpha1.VolumeGroupSnapshotClassList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.VolumeGroupSnapshotClassList{ListMeta: obj.(*v1alpha1.VolumeGroupSnapshotClassList).ListMeta}
	for _, item := range obj.(*v1alpha1.VolumeGroupSnapshotClassList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested volumeGroupSnapshotClasses.
func (c *FakeVolumeGroupSnapshotClasses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(volumegroupsnapshotclassesResource, opts))
}

// Create takes the representation of a volumeGroupSnapshotClass and creates it.  Returns the server's representation of the volumeGroupSnapshotClass, and an error, if there is any.
func (c *FakeVolumeGroupSnapshotClasses) Create(ctx context.Context, volumeGroupSnapshotClass *v1alpha1.VolumeGroupSnapshotClass, opts v1.CreateOptions) (result *v1alpha1.VolumeGroupSnapshotClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(volumegroupsnapshotclassesResource, volumeGroupSnapshotClass), &v1alpha1.VolumeGroupSnapshotClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VolumeGroupSnapshotClass), err
}

// Update takes the representation of a volumeGroupSnapshotClass and updates it. Returns the server's representation of the volumeGroupSnapshotClass, and an error, if there is any.
func (c *FakeVolumeGroupSnapshotClasses) Update(ctx context.Context, volumeGroupSnapshotClass *v1alpha1.VolumeGroupSnapshotClass, opts v1.UpdateOptions) (result *v1alpha1.VolumeGroupSnapshotClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(volumegroupsnapshotclassesResource, volumeGroupSnapshotClass), &v1alpha1.VolumeGroupSnapshotClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VolumeGroupSnapshotClass), err
}

// Delete takes name of the volumeGroupSnapshotClass and deletes it. Returns an error if one occurs.
func (c *FakeVolumeGroupSnapshotClasses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(volumegroupsnapshotclassesResource, name, opts), &v1alpha1.VolumeGroupSnapshotClass{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVolumeGroupSnapshotClasses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(volumegroupsnapshotclassesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.VolumeGroupSnapshotClassList{})
	return err
}

// Patch applies the patch and returns the patched volumeGroupSnapshotClass.
func (c *FakeVolumeGroupSnapshotClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.VolumeGroupSnapshotClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(volumegroupsnapshotclassesResource, name, pt, data, subresources...), &v1alpha1.VolumeGroupSnapshotClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VolumeGroupSnapshotClass), err
}
