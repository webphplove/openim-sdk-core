package db

import (
	"open_im_sdk/pkg/db/model_struct"
	"open_im_sdk/pkg/utils"
)

func (d *DataBase) GetMaxVersionKey(prefixedID string) (string, int32, error) {
	key := model_struct.LocalEncryptionKey{}
	err := d.conn.Model(model_struct.LocalEncryptionKey{}).Select("IFNULL(max(key_version),0)").Where("prefixed_id=?", prefixedID).Find(&key).Error
	return key.Key, key.KeyVersion, utils.Wrap(err, "GetMaxVersionKey")
}

func (d *DataBase) GetKey(prefixedID string, version int32) (string, error) {
	key := model_struct.LocalEncryptionKey{}
	err := d.conn.Model(model_struct.LocalEncryptionKey{}).Where("prefixed_id=? and key_version=?", prefixedID, version).Find(&key).Error
	return key.Key, utils.Wrap(err, "GetKey")
}
