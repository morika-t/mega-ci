package awsdeployer_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry/mega-ci/scripts/ci/deploy-aws-manifests/awsdeployer"
	"github.com/cloudfoundry/mega-ci/scripts/ci/deploy-aws-manifests/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AWSDeployer", func() {
	Describe("Deploy", func() {
		var (
			manifestsDirectory string
			fakeBOSH           *fakes.BOSH
			fakeSubnetChecker  *fakes.SubnetChecker
			awsDeployer        awsdeployer.AWSDeployer
		)

		BeforeEach(func() {
			fakeBOSH = new(fakes.BOSH)
			fakeSubnetChecker = new(fakes.SubnetChecker)

			var err error
			manifestsDirectory, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			awsDeployer = awsdeployer.NewAWSDeployer(fakeBOSH, fakeSubnetChecker)
			fakeSubnetChecker.CheckSubnetsCall.Returns.Bool = true
		})

		AfterEach(func() {
			os.RemoveAll(manifestsDirectory)
		})

		It("deploys all of the bosh manifests in the manifests directory", func() {
			writeManifest(manifestsDirectory, "first_manifest.yml")
			writeManifest(manifestsDirectory, "second_manifest.yml")
			writeManifest(manifestsDirectory, "not_a_manifest.json")

			deploymentError := awsDeployer.Deploy(manifestsDirectory)
			Expect(deploymentError).NotTo(HaveOccurred())

			Expect(len(fakeBOSH.DeployCalls.Receives.Manifest)).To(Equal(2))
			Expect(fakeBOSH.DeployCalls.Receives.Manifest[0]).To(ContainSubstring("first_manifest.yml"))
			Expect(fakeBOSH.DeployCalls.Receives.Manifest[1]).To(ContainSubstring("second_manifest.yml"))
		})

		It("replaces the bosh director uuid before deploying each manifest", func() {
			writeManifest(manifestsDirectory, "manifest.yml")
			fakeBOSH.StatusCall.Returns.UUID = "retrieved-director-uuid"
			deploymentError := awsDeployer.Deploy(manifestsDirectory)

			Expect(deploymentError).NotTo(HaveOccurred())

			deployedManifest, err := ioutil.ReadFile(fakeBOSH.DeployCalls.Receives.Manifest[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(string(deployedManifest)).To(ContainSubstring("director_uuid: retrieved-director-uuid"))
		})

		It("deletes the deployment", func() {
			writeManifestWithBody(manifestsDirectory, "manifest.yml", "director_uuid: BOSH-DIRECTOR-UUID\nname: some-deployment-name")

			deploymentError := awsDeployer.Deploy(manifestsDirectory)

			Expect(deploymentError).NotTo(HaveOccurred())
			Expect(fakeBOSH.DeleteDeploymentCall.Receives.DeploymentName).To(Equal("some-deployment-name"))
		})

		Context("failure cases", func() {
			It("returns an error when the BOSH manifest contains subnets not found on AWS", func() {
				const manifestWithSubnetC8 = `---
director_uuid: BOSH-DIRECTOR-UUID

name: multi-az-ssl

networks:
- subnets:
  - cloud_properties:
      subnet: "subnet-c8b76f90"
    range: 10.0.20.0/24
`
				writeManifestWithBody(manifestsDirectory, "manifest.yml", manifestWithSubnetC8)
				fakeSubnetChecker.CheckSubnetsCall.Returns.Bool = false

				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError).NotTo(BeNil())
				Expect(deploymentError.Error()).To(ContainSubstring("manifest subnets not found on AWS"))
			})

			It("returns an error when CheckSubnets returns an error", func() {
				writeManifest(manifestsDirectory, "manifest.yml")
				fakeSubnetChecker.CheckSubnetsCall.Returns.Error = errors.New("something bad happened")

				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError).NotTo(BeNil())
				Expect(deploymentError.Error()).To(ContainSubstring("something bad happened"))
			})

			It("returns an error when manifests directory does not exist", func() {
				deploymentError := awsDeployer.Deploy("/not/a/real/directory")
				Expect(deploymentError.Error()).To(ContainSubstring("no such file or directory"))
			})

			It("returns an error when bosh deploy fails", func() {
				writeManifest(manifestsDirectory, "manifest.yml")

				fakeBOSH.DeployCalls.Returns.Error = errors.New("bosh deployment failed")

				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError.Error()).To(ContainSubstring("bosh deployment failed"))
			})

			It("returns an error when the manifest is not valid yaml", func() {
				writeManifestWithBody(manifestsDirectory, "invalid_manifest.yml", "not: valid: yaml:")
				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError.Error()).To(ContainSubstring("mapping values are not allowed in this context"))
			})

			It("returns an error when bosh status fails", func() {
				writeManifest(manifestsDirectory, "manifest.yml")

				fakeBOSH.StatusCall.Returns.Error = errors.New("bosh status failed")

				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError.Error()).To(ContainSubstring("bosh status failed"))
			})

			It("returns an error when the deployment name is not present in the manifest", func() {
				writeManifestWithBody(manifestsDirectory, "invalid_manifest.yml", "director_uuid: BOSH-DIRECTOR-UUID")
				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError.Error()).To(ContainSubstring("deployment name missing from manifest"))
			})

			It("returns an error when deletion of the deployment fails", func() {
				writeManifestWithBody(manifestsDirectory, "manifest.yml", "director_uuid: BOSH-DIRECTOR-UUID\nname: some-deployment-name")
				fakeBOSH.DeleteDeploymentCall.Returns.Error = errors.New("failed to delete deployment: some-deployment-name")

				deploymentError := awsDeployer.Deploy(manifestsDirectory)
				Expect(deploymentError.Error()).To(ContainSubstring("failed to delete deployment: some-deployment-name"))
			})
		})
	})
})

func writeManifest(directory string, filename string) {
	writeManifestWithBody(directory, filename, "director_uuid: BOSH-DIRECTOR-UUID\nname: a-deployment-name")
}

func writeManifestWithBody(directory string, filename string, body string) {
	err := ioutil.WriteFile(filepath.Join(directory, filename), []byte(body), os.ModePerm)
	Expect(err).NotTo(HaveOccurred())
}