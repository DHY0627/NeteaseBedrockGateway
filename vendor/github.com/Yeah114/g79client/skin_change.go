package g79client

import "errors"

// 更换皮肤
func (c *Client) ChangeSkin(itemID string) error {
	purchaseResponse, err := c.PurchaseItem(itemID)
	if err != nil {
		return err
	}
	if purchaseResponse.Code != 0 && purchaseResponse.Code != 502 {
		return errors.New(purchaseResponse.Message)
	}

	_, err = c.SetUserSettingList(itemID)
	if err != nil {
		return err
	}
	return nil
}
