package main

import (
	"encoding/base64"
	"os"

	"github.com/pulumi/pulumi-azure/sdk/v6/go/azure/compute"
	"github.com/pulumi/pulumi-azure/sdk/v6/go/azure/core"
	"github.com/pulumi/pulumi-azure/sdk/v6/go/azure/network"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		conf := config.New(ctx, "")
		location := conf.Get("location")
		if location == "" {
			location = "southindia"
		}
		vmSize := conf.Get("vmSize")
		if vmSize == "" {
			vmSize = "Standard_B2ms"
		}

		publicKeyBytes, err := os.ReadFile("azure_rsa.pub")
		if err != nil {
			return err
		}
		publicKey := string(publicKeyBytes)

		resourceGroup, err := core.NewResourceGroup(ctx, "k8s-rg", &core.ResourceGroupArgs{
			Location: pulumi.String(location),
		})
		if err != nil {
			return err
		}
		vnet, err := network.NewVirtualNetwork(ctx, "k8s-vnet", &network.VirtualNetworkArgs{
			ResourceGroupName: resourceGroup.Name,
			AddressSpaces:     pulumi.StringArray{pulumi.String("10.0.0.0/16")},
		})
		if err != nil {
			return err
		}

		subnet, err := network.NewSubnet(ctx, "k8s-subnet", &network.SubnetArgs{
			ResourceGroupName:  resourceGroup.Name,
			VirtualNetworkName: vnet.Name,
			AddressPrefixes:    pulumi.StringArray{pulumi.String("10.0.1.0/24")},
		})
		if err != nil {
			return err
		}

		publicIP, err := network.NewPublicIp(ctx, "k8s-publicip", &network.PublicIpArgs{
			ResourceGroupName: resourceGroup.Name,
			AllocationMethod:  pulumi.String("Static"),
		})
		if err != nil {
			return err
		}

		// Allow SSH and K8s API ports (22, 16443)
		nsg, err := network.NewNetworkSecurityGroup(ctx, "k8s-nsg", &network.NetworkSecurityGroupArgs{
			ResourceGroupName: resourceGroup.Name,
			SecurityRules: network.NetworkSecurityGroupSecurityRuleArray{
				&network.NetworkSecurityGroupSecurityRuleArgs{
					Name: pulumi.String("SSH"), Priority: pulumi.Int(100), Direction: pulumi.String("Inbound"),
					Access: pulumi.String("Allow"), Protocol: pulumi.String("Tcp"),
					SourcePortRange: pulumi.String("*"), DestinationPortRange: pulumi.String("22"),
					SourceAddressPrefix: pulumi.String("*"), DestinationAddressPrefix: pulumi.String("*"),
				},
				&network.NetworkSecurityGroupSecurityRuleArgs{
					Name: pulumi.String("K8sAPI"), Priority: pulumi.Int(101), Direction: pulumi.String("Inbound"),
					Access: pulumi.String("Allow"), Protocol: pulumi.String("Tcp"),
					SourcePortRange: pulumi.String("*"), DestinationPortRange: pulumi.String("16443"),
					SourceAddressPrefix: pulumi.String("*"), DestinationAddressPrefix: pulumi.String("*"),
				},
				&network.NetworkSecurityGroupSecurityRuleArgs{
					Name: pulumi.String("NodePort"), Priority: pulumi.Int(102), Direction: pulumi.String("Inbound"),
					Access: pulumi.String("Allow"), Protocol: pulumi.String("Tcp"),
					SourcePortRange: pulumi.String("*"), DestinationPortRange: pulumi.String("30000"),
					SourceAddressPrefix: pulumi.String("*"), DestinationAddressPrefix: pulumi.String("*"),
				},
			},
		})
		if err != nil {
			return err
		}

		nic, err := network.NewNetworkInterface(ctx, "k8s-nic", &network.NetworkInterfaceArgs{
			ResourceGroupName: resourceGroup.Name,
			IpConfigurations: network.NetworkInterfaceIpConfigurationArray{
				&network.NetworkInterfaceIpConfigurationArgs{
					Name:                       pulumi.String("internal"),
					SubnetId:                   subnet.ID(),
					PrivateIpAddressAllocation: pulumi.String("Dynamic"),
					PublicIpAddressId:          publicIP.ID(),
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = network.NewNetworkInterfaceSecurityGroupAssociation(ctx, "nic-nsg-assoc", &network.NetworkInterfaceSecurityGroupAssociationArgs{
			NetworkInterfaceId:     nic.ID(),
			NetworkSecurityGroupId: nsg.ID(),
		})
		if err != nil {
			return err
		}

		// Cloud-init startup script
		cloudInit := `#!/bin/bash
		apt-get update
		snap install microk8s --classic --channel=1.29
		usermod -a -G microk8s azureuser
		microk8s status --wait-ready
		microk8s enable dns dashboard
		# Create a config file we can fetch later
		microk8s config > /home/azureuser/client.config
		chmod 600 /home/azureuser/client.config
		chown azureuser:azureuser /home/azureuser/client.config
		`
		_, err = compute.NewLinuxVirtualMachine(ctx, "k8s-vm", &compute.LinuxVirtualMachineArgs{
			ResourceGroupName: resourceGroup.Name,
			Size:              pulumi.String(vmSize),
			AdminUsername:     pulumi.String("azureuser"),
			NetworkInterfaceIds: pulumi.StringArray{
				nic.ID(),
			},
			AdminSshKeys: compute.LinuxVirtualMachineAdminSshKeyArray{
				&compute.LinuxVirtualMachineAdminSshKeyArgs{
					Username:  pulumi.String("azureuser"),
					PublicKey: pulumi.String(publicKey),
				},
			},
			OsDisk: &compute.LinuxVirtualMachineOsDiskArgs{
				Caching:            pulumi.String("ReadWrite"),
				StorageAccountType: pulumi.String("Standard_LRS"),
			},
			SourceImageReference: &compute.LinuxVirtualMachineSourceImageReferenceArgs{
				Publisher: pulumi.String("Canonical"),
				Offer:     pulumi.String("0001-com-ubuntu-server-jammy"),
				Sku:       pulumi.String("22_04-lts"),
				Version:   pulumi.String("latest"),
			},
			CustomData: pulumi.String(base64.StdEncoding.EncodeToString([]byte(cloudInit))),
		})
		if err != nil {
			return err
		}

		ctx.Export("publicIp", publicIP.IpAddress)
		return nil
	})
}
