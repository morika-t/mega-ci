package awsdeployer

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v2"

	"github.com/cloudfoundry/mega-ci/scripts/ci/deploy-aws-manifests/clients"
	"github.com/cloudfoundry/mega-ci/scripts/ci/deploy-aws-manifests/manifests"
)

type SubnetChecker interface {
	CheckSubnets(manifestFilename string) (bool, error)
}

type AWSDeployer struct {
	bosh          clients.BOSH
	subnetChecker SubnetChecker
	stdout        io.Writer
}

func NewAWSDeployer(bosh clients.BOSH, subnetChecker SubnetChecker, stdout io.Writer) AWSDeployer {
	return AWSDeployer{
		bosh:          bosh,
		subnetChecker: subnetChecker,
		stdout:        stdout,
	}
}

func (a AWSDeployer) Deploy(manifestFilename string) error {
	fmt.Fprintf(a.stdout, "deploying manifest: %s\n", manifestFilename)
	fmt.Fprintln(a.stdout, "checking subnets...")
	hasSubnets, err := a.subnetChecker.CheckSubnets(manifestFilename)
	if err != nil {
		return err
	}

	if !hasSubnets {
		return errors.New("manifest subnets not found on AWS")
	}

	fmt.Fprintln(a.stdout, "found all manifest subnets on AWS")

	err = a.deployManifest(manifestFilename)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, "\n\n")

	return nil
}

func (a AWSDeployer) deployManifest(manifestFilename string) error {
	fmt.Fprintln(a.stdout, "fetching director uuid...")
	manifest, err := a.replaceUUID(manifestFilename)
	if err != nil {
		return err
	}

	buf, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}

	fmt.Fprintln(a.stdout, "deploying...")
	err = a.bosh.Deploy(buf)
	if err != nil {
		return err
	}

	fmt.Fprintln(a.stdout, "deleting deployment...")
	err = a.deleteDeployment(manifest)
	if err != nil {
		return err
	}

	return nil
}

func (a AWSDeployer) replaceUUID(manifestFilename string) (map[string]interface{}, error) {
	directorUUID, err := a.bosh.UUID()
	if err != nil {
		return nil, err
	}

	manifest, err := manifests.ReadManifest(manifestFilename)
	if err != nil {
		return nil, err
	}

	manifest["director_uuid"] = directorUUID

	return manifest, nil
}

func (a AWSDeployer) deleteDeployment(manifest map[string]interface{}) error {
	deploymentName, ok := manifest["name"].(string)
	if !ok {
		return errors.New("deployment name missing from manifest")
	}

	err := a.bosh.DeleteDeployment(deploymentName)
	if err != nil {
		return err
	}

	return nil
}
