package appstore

import (
	"context"
	"errors"

	"github.com/majd/ipatool/v2/internal/store"
)

// QingzhouCheckAuthentication exercises the bag, resource loader and SAP signing
// session without sending an Apple ID, password or authentication request.
func QingzhouCheckAuthentication(client AppStore) error {
	t, ok := client.(*appstore)
	if !ok {
		return errors.New("unsupported authentication client")
	}
	mac, err := t.machine.MacAddress()
	if err != nil {
		return err
	}
	guid, machineID, err := machineIdentity(mac)
	if err != nil {
		return err
	}
	bag, err := t.bag(guid)
	if err != nil {
		return errors.New("failed to get bag")
	}
	signer, err := t.actionSignerFactory(bag.SAPConfig, machineID)
	if err != nil {
		return err
	}
	defer signer.Close()
	signature, err := signer.Sign([]byte("Qingzhou authentication environment check"))
	if err != nil {
		return err
	}
	if len(signature) == 0 {
		return errors.New("SAP signing returned no signature")
	}
	return store.CheckMachineProof(context.Background(), guid)
}
