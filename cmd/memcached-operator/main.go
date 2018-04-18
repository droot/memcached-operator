package main

import (
	"context"
	"runtime"

	sdk "github.com/coreos/operator-sdk/pkg/sdk"
	sdkVersion "github.com/coreos/operator-sdk/version"
	stub "github.com/droot/memcached-operator/pkg/stub"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()
	sdk.Watch("memcached.example.com/v1alpha1", "Memcached", "default", 5)
	sdk.Watch("v1", "Pod", "default", 5)
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
