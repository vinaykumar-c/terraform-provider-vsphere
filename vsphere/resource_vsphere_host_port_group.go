package vsphere

import (
	"fmt"
    "log"
	"context"
    "github.com/vmware/govmomi/find"
    "github.com/hashicorp/terraform-provider-vsphere/vsphere/internal/helper/provider"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-provider-vsphere/vsphere/internal/helper/structure"
	"github.com/vmware/govmomi/object"
)

func resourceVSphereHostPortGroup() *schema.Resource {
	s := map[string]*schema.Schema{
		"host_system_id": {
			Type:        schema.TypeString,
			Description: "The managed object ID of the host to set the virtual switch up on.",
			Required:    true,
			ForceNew:    true,
		},
		"datacenter_id": {
			Type:        schema.TypeString,
			Description: "The managed object ID of the datacenter to set the virtual switch up on.",
			Required:    true,
			ForceNew:    true,
		},
		"computed_policy": {
			Type:        schema.TypeMap,
			Description: "The effective network policy after inheritance. Note that this will look similar to, but is not the same, as the policy attributes defined in this resource.",
			Computed:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
		},
		"key": {
			Type:        schema.TypeString,
			Description: "The linkable identifier for this port group.",
			Computed:    true,
		},
		"ports": {
			Type:        schema.TypeList,
			Description: "The ports that currently exist and are used on this port group.",
			Computed:    true,
			MaxItems:    1,
			Elem:        portGroupPortSchema(),
		},
	}
	structure.MergeSchema(s, schemaHostPortGroupSpec())

	// Transform any necessary fields in the schema that need to be updated
	// specifically for this resource.
	s["active_nics"].Optional = true
	s["standby_nics"].Optional = true

	return &schema.Resource{
		Create: resourceVSphereHostPortGroupCreate,
		Read:   resourceVSphereHostPortGroupRead,
		Update: resourceVSphereHostPortGroupUpdate,
		Delete: resourceVSphereHostPortGroupDelete,
		Schema: s,
	}
}

func resourceVSphereHostPortGroupCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	name := d.Get("name").(string)
	hsID := d.Get("host_system_id").(string)
	ns, err := hostNetworkSystemFromHostSystemID(client, hsID)
	if err != nil {
		return fmt.Errorf("error loading network system: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	spec := expandHostPortGroupSpec(d)
	if err := ns.AddPortGroup(ctx, *spec); err != nil {
		return fmt.Errorf("error adding port group: %s", err)
	}

	saveHostPortGroupID(d, hsID, name)
	return resourceVSphereHostPortGroupRead(d, meta)
}

func resourceVSphereHostPortGroupRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	name := d.Get("name").(string)
	hsID := d.Get("host_system_id").(string)
	//hsID, name, err := portGroupIDsFromResourceID(d)
	//if err != nil {
	//	return err
	//}
	ns, err := hostNetworkSystemFromHostSystemID(client, hsID)
	if err != nil {
		return fmt.Errorf("error loading host network system: %s", err)
	}

	pg, err := hostPortGroupFromName(meta.(*VSphereClient).vimClient, ns, name)
	if err != nil {
		return fmt.Errorf("error fetching port group data: %s", err)
	}

	if err := flattenHostPortGroupSpec(d, &pg.Spec); err != nil {
		return fmt.Errorf("error setting resource data: %s", err)
	}

	d.Set("key", pg.Key)

	var dc *object.Datacenter
	if dcID, ok := d.GetOk("datacenter_id"); ok {
		var err error
		dc, err = datacenterFromID(client, dcID.(string))
		if err != nil {
			return fmt.Errorf("cannot locate datacenter: %s", err)
		}
	}

	finder := find.NewFinder(client.Client, false)
	if dc != nil {
		finder.SetDatacenter(dc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), provider.DefaultAPITimeout)
	defer cancel()
	networks, err := finder.NetworkList(ctx, name)
	if err != nil {
		return err
	}
	if len(networks) == 0 {
		return fmt.Errorf("%s %s not found", "Network", name)
	}

    d.SetId(networks[0].Reference().Value)
	log.Printf("[DEBUG] Network ID is %s", networks[0].Reference().Value)
	cpm, err := calculateComputedPolicy(pg.ComputedPolicy)
	if err != nil {
		return err
	}
	if err := d.Set("computed_policy", cpm); err != nil {
		return fmt.Errorf("error saving effective policy to state: %s", err)
	}
	if err := d.Set("ports", calculatePorts(pg.Port)); err != nil {
		return fmt.Errorf("error setting port list: %s", err)
	}

	return nil
}

func resourceVSphereHostPortGroupUpdate(d *schema.ResourceData, meta interface{}) error {
	name := d.Get("name").(string)
	hsID := d.Get("host_system_id").(string)
	//saveHostPortGroupID(d, hsID, name)
	client := meta.(*VSphereClient).vimClient
	//hsID, name, err := portGroupIDsFromResourceID(d)
	//if err != nil {
	//	return err
	//}
	ns, err := hostNetworkSystemFromHostSystemID(client, hsID)
	if err != nil {
		return fmt.Errorf("error loading host network system: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	spec := expandHostPortGroupSpec(d)
	if err := ns.UpdatePortGroup(ctx, name, *spec); err != nil {
		return fmt.Errorf("error updating port group: %s", err)
	}

	return resourceVSphereHostPortGroupRead(d, meta)
}

func resourceVSphereHostPortGroupDelete(d *schema.ResourceData, meta interface{}) error {
	name := d.Get("name").(string)
	hsID := d.Get("host_system_id").(string)
	//saveHostPortGroupID(d, hsID, name)
	client := meta.(*VSphereClient).vimClient
	//hsID, name, err := portGroupIDsFromResourceID(d)
	//if err != nil {
	//	return err
	//}
	ns, err := hostNetworkSystemFromHostSystemID(client, hsID)
	if err != nil {
		return fmt.Errorf("error loading host network system: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	if err := ns.RemovePortGroup(ctx, name); err != nil {
		return fmt.Errorf("error deleting port group: %s", err)
	}

	return nil
}
