package encryption

//
//import (
//	"open_im_sdk/pkg/constant"
//	"open_im_sdk/pkg/db"
//	"open_im_sdk/pkg/server_api_params"
//	"open_im_sdk/pkg/utils"
//)
//
//type Encryption struct {
//	loginUserID   string
//	db            *db.DataBase
//	isNeedEncrypt bool
//}
//
//func (e *Encryption) SyncKeys(operationID string) {
//
//}
//
//func (e *Encryption) getKey(prefixedID string, version int32, operationID string) string {
//	key, _ := e.db.GetKey(prefixedID, version)
//
//	return key
//
//}
//
//func (e *Encryption) getMaxVersionKey(prefixedID string, operationID string) (string, int32) {
//	key, version, err := e.db.GetMaxVersionKey(prefixedID)
//	if err != nil {
//		return "", 0
//	}
//	return key, version
//}
//
//func (e *Encryption) getPrefixedID(userID string, groupID string) string {
//	if userID != "" {
//		return "u_" + userID
//	}
//	if groupID != "" {
//		return "g_" + groupID
//	}
//	return ""
//}
//
//func (e *Encryption) Decryption(msgList []*server_api_params.MsgData, operationID string) error {
//	if !e.isNeedEncrypt {
//		return nil
//	}
//	for _, wsMsgData := range msgList {
//		prefixedID := ""
//		key := ""
//		switch wsMsgData.SessionType {
//		case constant.SingleChatType:
//			prefixedID = e.getPrefixedID(e.loginUserID, "")
//			if wsMsgData.SendID == e.loginUserID {
//				key = e.getKey(prefixedID, wsMsgData.SendIDKeyVersion, operationID)
//			} else {
//				key = e.getKey(prefixedID, wsMsgData.RecvIDKeyVersion, operationID)
//			}
//
//		case constant.GroupChatType:
//			prefixedID = e.getPrefixedID(e.loginUserID, "")
//			key = e.getKey(prefixedID, wsMsgData.RecvIDKeyVersion, operationID)
//
//		case constant.SuperGroupChatType:
//			prefixedID = e.getPrefixedID("", wsMsgData.GroupID)
//			key = e.getKey(prefixedID, wsMsgData.SendIDKeyVersion, operationID)
//			ec, err := utils.AesDecrypt(wsMsgData.Content, []byte(key))
//			if err != nil {
//				return utils.Wrap(err, "AesDecrypt failed")
//			}
//			wsMsgData.Content = ec
//		}
//		dc, err := utils.AesDecrypt(wsMsgData.Content, []byte(key))
//		if err != nil {
//			return utils.Wrap(err, "AesDecrypt failed")
//		}
//		wsMsgData.Content = dc
//	}
//	return nil
//}
//
//func (e *Encryption) Encryption(wsMsgData *server_api_params.MsgData, operationID string) error {
//	if !e.isNeedEncrypt {
//		return nil
//	}
//	switch wsMsgData.SessionType {
//	case constant.SingleChatType, constant.GroupChatType:
//		prefixedID := e.getPrefixedID(wsMsgData.SendID, "")
//		key, version := e.getMaxVersionKey(prefixedID, operationID)
//		ec, err := utils.AesEncrypt(wsMsgData.Content, []byte(key))
//		if err != nil {
//			return utils.Wrap(err, "AesEncrypt failed")
//		}
//		wsMsgData.Content = ec
//		wsMsgData.SendIDKeyVersion = version
//	case constant.SuperGroupChatType:
//		prefixedID := e.getPrefixedID("", wsMsgData.GroupID)
//		key, version := e.getMaxVersionKey(prefixedID, operationID)
//		ec, err := utils.AesEncrypt(wsMsgData.Content, []byte(key))
//		if err != nil {
//			return utils.Wrap(err, "AesEncrypt failed")
//		}
//		wsMsgData.Content = ec
//		wsMsgData.SendIDKeyVersion = version
//	}
//	return nil
//}
