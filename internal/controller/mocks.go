/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockClient is a mock implementation of the client.Client interface for testing
type MockClient struct {
	client.Client
	getFunc    func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	listFunc   func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	createFunc func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	updateFunc func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	deleteFunc func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
	scheme     *runtime.Scheme
}

// Get provides a mock for k8s client get() method
func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj, opts...)
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

// List provides a mock for k8s client list() method
func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if m.listFunc != nil {
		return m.listFunc(ctx, list, opts...)
	}
	return m.Client.List(ctx, list, opts...)
}

// Create provides a mock for k8s client create() method
func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, obj, opts...)
	}
	return m.Client.Create(ctx, obj, opts...)
}

// Update provides a mock for k8s client update() method
func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, obj, opts...)
	}
	return m.Client.Update(ctx, obj, opts...)
}

// Delete provides a mock for k8s client delete() method
func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, obj, opts...)
	}
	return m.Client.Delete(ctx, obj, opts...)
}

// Scheme returns the mock client's scheme
func (m *MockClient) Scheme() *runtime.Scheme {
	return m.scheme
}

// FakeEventRecorder is a simple fake event recorder for testing
type FakeEventRecorder struct {
	Events []string
}

// Event records a normal event with the given type, reason and message
func (f *FakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	if f.Events == nil {
		f.Events = []string{}
	}
	f.Events = append(f.Events, eventtype+" "+reason+" "+message)
}

// Eventf records a normal event with the given type, reason and formatted message
func (f *FakeEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Event(object, eventtype, reason, messageFmt)
}

// AnnotatedEventf records an event with annotations
func (f *FakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Event(object, eventtype, reason, messageFmt)
}
