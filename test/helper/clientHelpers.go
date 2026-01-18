package helper

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientTestContext struct {
	Ctx    context.Context
	Client client.Client
}
