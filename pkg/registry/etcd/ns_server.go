// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"context"
	"errors"
	"io"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	v1 "github.com/networkservicemesh/sdk-k8s/pkg/tools/k8s/apis/networkservicemesh.io/v1"
	"github.com/networkservicemesh/sdk-k8s/pkg/tools/k8s/client/clientset/versioned"
	"github.com/networkservicemesh/sdk/pkg/registry/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/matchutils"
)

type etcdNSRegistryServer struct {
	chainContext context.Context
	client       versioned.Interface
	ns           string
}

func (n *etcdNSRegistryServer) Register(ctx context.Context, request *registry.NetworkService) (*registry.NetworkService, error) {
	resp, err := n.client.NetworkservicemeshV1().NetworkServices(n.ns).Create(
		ctx,
		&v1.NetworkService{
			ObjectMeta: metav1.ObjectMeta{
				Name: request.Name,
			},
			Spec: *(*v1.NetworkServiceSpec)(request),
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, err
	}
	resp.Spec.DeepCopyInto((*v1.NetworkServiceSpec)(request))
	request.Name = resp.Name
	return next.NetworkServiceRegistryServer(ctx).Register(ctx, request)
}

func (n *etcdNSRegistryServer) watch(query *registry.NetworkServiceQuery, s registry.NetworkServiceRegistry_FindServer) error {
	watcher, err := n.client.NetworkservicemeshV1().NetworkServices(n.ns).Watch(s.Context(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for {
		select {
		case <-s.Context().Done():
			return s.Context().Err()
		case event := <-watcher.ResultChan():
			if event.Type != watch.Added {
				continue
			}
			model := event.Object.(*v1.NetworkService)
			item := (*registry.NetworkService)(&model.Spec)
			if matchutils.MatchNetworkServices(query.NetworkService, item) {
				err := s.Send(item)
				if err != nil {
					return err
				}
			}
		}
	}
}

func (n *etcdNSRegistryServer) Find(query *registry.NetworkServiceQuery, s registry.NetworkServiceRegistry_FindServer) error {
	if query.Watch {
		if err := n.watch(query, s); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	} else {
		list, err := n.client.NetworkservicemeshV1().NetworkServices(n.ns).List(s.Context(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		for i := 0; i < len(list.Items); i++ {
			item := (*registry.NetworkService)(&list.Items[i].Spec)
			if matchutils.MatchNetworkServices(query.NetworkService, item) {
				err := s.Send(item)
				if err != nil {
					return err
				}
			}
		}
	}
	return next.NetworkServiceRegistryServer(s.Context()).Find(query, s)
}

func (n *etcdNSRegistryServer) Unregister(ctx context.Context, request *registry.NetworkService) (*empty.Empty, error) {
	err := n.client.NetworkservicemeshV1().NetworkServices(n.ns).Delete(
		ctx,
		request.Name,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return nil, err
	}
	return next.NetworkServiceRegistryServer(ctx).Unregister(ctx, request)
}

// NewNetworkServiceRegistryServer creates new registry.NetworkServiceRegistryServer that is using etcd to store network services.
func NewNetworkServiceRegistryServer(chainContext context.Context, ns string, client versioned.Interface) registry.NetworkServiceRegistryServer {
	return &etcdNSRegistryServer{
		chainContext: chainContext,
		client:       client,
		ns:           ns,
	}
}
