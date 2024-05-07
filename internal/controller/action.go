package controller

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Action interface {
	Execute(ctx context.Context) error
}

var (
	_ Action = &PatchStateus{}
	_ Action = &CreateObject{}
)

type PatchStateus struct {
	client.Client
	original client.Object
	new      client.Object
}

func (p *PatchStateus) Execute(ctx context.Context) error {
	if reflect.DeepEqual(p.original, p.new) {
		return nil
	}
	if err := p.Client.Status().Patch(ctx, p.new, client.MergeFrom(p.original)); err != nil {
		return fmt.Errorf("failed to patch status: %w", err)
	}
	return nil
}

type CreateObject struct {
	client.Client
	obj client.Object
}

func (c *CreateObject) Execute(ctx context.Context) error {
	if err := c.Client.Create(ctx, c.obj); err != nil {
		return fmt.Errorf("failed to create object: %w", err)
	}
	return nil
}
