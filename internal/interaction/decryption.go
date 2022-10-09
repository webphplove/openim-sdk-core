package interaction

import (
	"errors"
	"open_im_sdk/pkg/constant"
	"open_im_sdk/pkg/log"
	"open_im_sdk/pkg/server_api_params"
	api "open_im_sdk/pkg/server_api_params"
	"open_im_sdk/pkg/utils"
	"open_im_sdk/sdk_struct"
	"sync"
)

type KeyMsg struct {
	LoginUserID     string
	IDVersionKey    map[string]string          //g_groupid_version->key; u_userid_version->key
	IDMaxVersionKey map[string]*api.VersionKey //g_groupid->max_version, key; u_userid->max_version, key
	IDVersionKeyMtx sync.Mutex
	postApi         *PostApi
	config          *sdk_struct.IMConfig
}

//对外  加密 发送消息时调用
func (k *KeyMsg) EncryptMsg(wsMsgData *server_api_params.MsgData, operationID string) error {
	if k.config.IsNeedEncryption {
		var err error
		var key *api.VersionKey
		if wsMsgData.ContentType != constant.Text {
			log.Error(operationID, "wsMsgData.ContentType is not Text ", wsMsgData.ContentType)
			return nil
		}
		if wsMsgData.SessionType == constant.SuperGroupChatType {
			err, key = k.GetMaxVersionKey("", wsMsgData.GroupID, operationID)
			if err != nil {
				log.Error(operationID, "GetMaxVersionKey failed ", err.Error(), wsMsgData.GroupID)
				return utils.Wrap(err, "")
			}
		} else {
			err, key = k.GetMaxVersionKey(k.LoginUserID, "", operationID)
			if err != nil {
				log.Error(operationID, "GetMaxVersionKey failed ", err.Error(), k.LoginUserID)
				return utils.Wrap(err, "")
			}
		}
		encryptContent, err := utils.AesEncrypt(wsMsgData.Content, []byte(key.Key))
		if err != nil {
			log.Error(operationID, "AesEncrypt failed ", err.Error())
			return utils.Wrap(err, "")
		}
		log.Info(operationID, "AesEncrypt ok ", "key ", *key, wsMsgData.SessionType, k.LoginUserID, wsMsgData.GroupID)
		wsMsgData.Content = encryptContent
		wsMsgData.KeyVersion = key.Version
	}
	return nil
}

//对外
func NewKeyMsg(loginUserID string, p *PostApi, config *sdk_struct.IMConfig) *KeyMsg {
	k := &KeyMsg{LoginUserID: loginUserID, postApi: p, config: config}
	k.IDVersionKey = make(map[string]string, 0)
	k.IDMaxVersionKey = make(map[string]*api.VersionKey, 0)
	go func() {
		operationID := utils.OperationIDGenerator()
		_, err := k.getEncryptionKeyFromSvr(operationID, k.LoginUserID, "", 0)
		if err != nil {
			log.Error(operationID, "getEncryptionKeyFromSvr failed ", err.Error(), k.LoginUserID)
		}
		_, err = k.getAllJoinedGroupEncryptionKeyReqFromSvr(operationID)
		if err != nil {
			log.Error(operationID, "getAllJoinedGroupEncryptionKeyReqFromSvr failed ", err.Error(), k.LoginUserID)
		}
	}()
	return k
}

//1
func (k *KeyMsg) updateUserMaxVersion(userID string, versionKeyList []*api.VersionKey, operationID string) {
	k.IDVersionKeyMtx.Lock()
	defer k.IDVersionKeyMtx.Unlock()
	for _, v := range versionKeyList {
		vk, ok := k.IDMaxVersionKey[k.genUserIDPrefixKey(userID)]
		if ok {
			if v.Version > vk.Version {
				k.IDMaxVersionKey[k.genUserIDPrefixKey(userID)] = v
			}
		} else {
			k.IDMaxVersionKey[k.genUserIDPrefixKey(userID)] = v
		}
	}
}

//1
func (k *KeyMsg) updateGroupMaxVersion(versionKeyList []*api.GroupVersionKey, operationID string) {
	k.IDVersionKeyMtx.Lock()
	defer k.IDVersionKeyMtx.Unlock()
	for _, v := range versionKeyList {
		vk, ok := k.IDMaxVersionKey[k.genGroupIDPrefixKey(v.GroupID)]
		if ok {
			if v.Version > vk.Version {
				k.IDMaxVersionKey[k.genGroupIDPrefixKey(v.GroupID)] = &api.VersionKey{Version: v.Version, Key: v.Key}
			}
		} else {
			k.IDMaxVersionKey[k.genGroupIDPrefixKey(v.GroupID)] = &api.VersionKey{Version: v.Version, Key: v.Key}
		}

	}
}

//对外 1 从内存获取指定版本（不为0）key  如不存在则从服务端获取 并更新内存
func (k *KeyMsg) GetKeyByVersion(userID string, groupID string, version int32, operationID string) (error, string) {
	log.Info(operationID, utils.GetSelfFuncName(), " args: ", userID, groupID, version)
	if (userID == "" && groupID == "") || version == 0 {
		return errors.New("args failed "), ""
	}
	if userID != "" {
		k.IDVersionKeyMtx.Lock()
		key, ok := k.IDVersionKey[k.genUserIDVersionKey(userID, version)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			log.Info(operationID, "key in map: ", userID, version, key)
			return nil, key
		}
		k.IDVersionKeyMtx.Unlock()
		log.Info(operationID, "key not in map: ", userID, version)
		keyList, err := k.getEncryptionKeyFromSvr(operationID, userID, "", version)
		if err != nil {
			log.Error(operationID, "getEncryptionKeyFromSvr failed ", err.Error(), userID, version)
			return utils.Wrap(err, ""), ""
		}
		log.Info(operationID, "key from svr: ", userID, version, keyList[0].Key)
		return nil, keyList[0].Key
	}
	if groupID != "" {
		k.IDVersionKeyMtx.Lock()
		key, ok := k.IDVersionKey[k.genGroupIDVersionKey(groupID, version)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			log.Info(operationID, "key in map: ", groupID, version, key)
			return nil, key
		}
		k.IDVersionKeyMtx.Unlock()
		log.Info(operationID, "key not in map: ", groupID, version)
		keyList, err := k.getEncryptionKeyFromSvr(operationID, "", groupID, version)
		if err != nil {
			log.Error(operationID, "getEncryptionKeyFromSvr failed ", err.Error(), groupID, version)
			return utils.Wrap(err, ""), ""
		}
		k.IDVersionKey[k.genGroupIDVersionKey(groupID, version)] = keyList[0].Key
		log.Info(operationID, "key from svr: ", groupID, version, keyList[0].Key)
		return nil, keyList[0].Key
	}
	return errors.New("args failed"), ""
}

//1
func (k *KeyMsg) genUserIDVersionKey(userID string, version int32) string {
	return "u" + userID + "v" + utils.Int32ToString(version)
}

//1
func (k *KeyMsg) genGroupIDVersionKey(groupID string, version int32) string {
	return "g" + groupID + "v" + utils.Int32ToString(version)
}

//1
func (k *KeyMsg) genUserIDPrefixKey(userID string) string {
	return "u" + userID
}

//1
func (k *KeyMsg) genGroupIDPrefixKey(groupID string) string {
	return "g" + groupID
}

//对外 解密消息 收到推送消息或者主动拉取消息时调用
func (k *KeyMsg) DecryptMsg(msgList []*server_api_params.MsgData, operationID string) {
	log.Info(operationID, utils.GetSelfFuncName(), "args, msgList len: ", len(msgList))
	if !k.config.IsNeedEncryption {
		return
	}
	for _, msg := range msgList {
		log.Info(operationID, "msg key version: ", msg.KeyVersion, "msg: ", msg.String())
		if msg.KeyVersion != 0 && msg.ContentType == constant.Text {
			var err error
			var dContent []byte
			switch msg.SessionType {
			case constant.SingleChatType, constant.GroupChatType:
				if msg.RecvID != k.LoginUserID && msg.SendID != k.LoginUserID {
					log.Error(operationID, "malformed message ", msg.SendID, k.LoginUserID, msg.String())
					continue
				}
				err, dContent = k.decryptContent(k.LoginUserID, "", msg.KeyVersion, msg.Content, operationID)
				log.Info(operationID, "decryptContent args: userID: ", k.LoginUserID, msg.KeyVersion, " decrypt result: ", string(dContent))
			case constant.SuperGroupChatType:
				err, dContent = k.decryptContent("", msg.GroupID, msg.KeyVersion, msg.Content, operationID)
				log.Info(operationID, "decryptContent args: groupID: ", msg.GroupID, msg.KeyVersion, " decrypt result: ", string(dContent))
			}
			if err != nil {
				log.Error(operationID, "decryptContent failed ", err.Error(), k.LoginUserID, msg.GroupID, msg.SessionType, msg.String())
				continue
			}
			msg.Content = dContent
		}
	}
}

//1 对消息内容解密
func (k *KeyMsg) decryptContent(userID string, groupID string, version int32, encryptContent []byte, operationID string) (error, []byte) {
	log.Info(operationID, utils.GetSelfFuncName(), " args： ", userID, groupID, version)
	var err error
	var key string
	if userID != "" {
		err, key = k.GetKeyByVersion(userID, "", version, operationID)
		if err != nil {
			log.Error(operationID, "GetKeyByVersion failed ", err.Error(), userID, version)
			return utils.Wrap(err, ""), nil
		}
	}
	if groupID != "" {
		err, key = k.GetKeyByVersion("", groupID, version, operationID)
		if err != nil {
			log.Error(operationID, "GetKeyByVersion failed ", err.Error(), groupID, version)
			return utils.Wrap(err, ""), nil
		}
	}
	log.Info(operationID, "get key ", key, groupID, userID, version)
	decryptContent, err := utils.AesDecrypt(encryptContent, []byte(key))
	if err != nil {
		log.Error(operationID, "AesDecrypt failed ", err.Error())
		return utils.Wrap(err, ""), nil
	}
	log.Info(operationID, "AesDecrypt content: ", string(decryptContent))
	return nil, decryptContent
}

//1 从服务端获取userID或groupID (指定version 或者 所有version)加密key，并更新内存max version key和 version key
func (k *KeyMsg) getEncryptionKeyFromSvr(operationID string, userID string, groupID string, version int32) ([]*api.VersionKey, error) {
	log.Info(operationID, utils.GetSelfFuncName(), " args: ", userID, groupID, version)
	apiReq := api.GetEncryptionKeyReq{OperationID: operationID, UserID: userID, GroupID: groupID, KeyVersion: version}
	var versionKeyList []*api.VersionKey
	err := k.postApi.PostReturn(constant.GetEncryptionKeyRouter, apiReq, &versionKeyList)
	if err != nil {
		log.Error(operationID, "post api failed ", err.Error(), constant.GetEncryptionKeyRouter, apiReq)
		return nil, utils.Wrap(err, apiReq.OperationID)
	}
	log.Info(operationID, "post api ok ", apiReq, versionKeyList)
	if userID != "" {
		k.IDVersionKeyMtx.Lock()
		for _, v := range versionKeyList {
			k.IDVersionKey[k.genUserIDVersionKey(userID, v.Version)] = v.Key
		}
		k.IDVersionKeyMtx.Unlock()
		k.updateUserMaxVersion(userID, versionKeyList, operationID)
	}
	if groupID != "" {
		var groupVersionKeyList []*api.GroupVersionKey
		for _, v := range versionKeyList {
			groupVersionKeyList = append(groupVersionKeyList, &api.GroupVersionKey{Version: v.Version, Key: v.Key, GroupID: groupID})
		}
		k.IDVersionKeyMtx.Lock()
		for _, v := range versionKeyList {
			k.IDVersionKey[k.genGroupIDVersionKey(groupID, v.Version)] = v.Key
			log.Info(operationID, "set IDVersionKey ", groupID, v.Version, v.Key)
		}
		k.IDVersionKeyMtx.Unlock()
		k.updateGroupMaxVersion(groupVersionKeyList, operationID)
	}
	return versionKeyList, nil
}

//1 从服务端获取所有加入的groupID加密key，并更新内存key
func (k *KeyMsg) getAllJoinedGroupEncryptionKeyReqFromSvr(operationID string) ([]*api.GroupVersionKey, error) {
	apiReq := api.GetAllJoinedGroupEncryptionKeyReq{OperationID: operationID, UserID: k.LoginUserID}
	var groupVersionKeyList []*api.GroupVersionKey
	err := k.postApi.PostReturn(constant.GetAllJoinedGroupEncryptionKeyRouter, apiReq, &groupVersionKeyList)
	if err != nil {
		log.Error(operationID, "post api failed ", err.Error(), constant.GetAllJoinedGroupEncryptionKeyRouter, apiReq)
		return nil, utils.Wrap(err, apiReq.OperationID)
	}
	k.updateGroupMaxVersion(groupVersionKeyList, operationID)
	k.IDVersionKeyMtx.Lock()
	for _, v := range groupVersionKeyList {
		k.IDVersionKey[k.genGroupIDVersionKey(v.GroupID, v.Version)] = v.Key
	}
	k.IDVersionKeyMtx.Unlock()
	return groupVersionKeyList, nil
}

//对外 1 从服务端更新key到内存 新加入群，或者群key更新，或者用户key更新时调用
func (k *KeyMsg) UpdateKeyFromSvr(userID, groupID, operationID string) {
	k.getEncryptionKeyFromSvr(operationID, userID, groupID, 0)
	//
	//if userID != "" {
	//	_, err := k.getEncryptionKeyFromSvr(operationID, userID, "", 0)
	//	if err != nil {
	//		log.Error(operationID, "getEncryptionKeyFromSvr failed ", err.Error(), userID)
	//		return
	//	}
	//	//k.IDVersionKeyMtx.Lock()
	//	//defer k.IDVersionKeyMtx.Unlock()
	//	//for _, v := range versionKeyList {
	//	//	k.IDVersionKey[k.genUserIDVersionKey(userID, v.Version)] = v.Key
	//	//}
	//	//k.updateUserMaxVersionNoLock(userID, versionKeyList, operationID)
	//	return
	//}
	//if groupID != "" {
	//	_, err := k.getEncryptionKeyFromSvr(operationID, "", groupID, 0)
	//	if err != nil {
	//		log.Error(operationID, "getEncryptionKeyFromSvr failed ", err.Error(), groupID)
	//		return
	//	}
	//	//k.IDVersionKeyMtx.Lock()
	//	//defer k.IDVersionKeyMtx.Unlock()
	//	//for _, v := range versionKeyList {
	//	//	k.IDVersionKey[k.genGroupIDVersionKey(groupID, v.Version)] = v.Key
	//	//}
	//	//var versionGroupKeyList []*api.GroupVersionKey
	//	//for _, v := range versionKeyList {
	//	//	versionGroupKeyList = append(versionGroupKeyList, &api.GroupVersionKey{Version: v.Version, Key: v.Key, GroupID: groupID})
	//	//}
	//	//k.updateGroupMaxVersionNoLock(versionGroupKeyList, operationID)
	//	return
	//}
	//log.Error(operationID, "args failed ", userID, groupID)
}

//对外 1 从内存获取最大版本key，如果不存在则从服务端拉取并更新到内存， 发送消息时调用
func (k *KeyMsg) GetMaxVersionKey(userID, groupID, operationID string) (error, *api.VersionKey) {
	if userID != "" {
		k.IDVersionKeyMtx.Lock()
		v, ok := k.IDMaxVersionKey[k.genUserIDPrefixKey(userID)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			return nil, v
		}
		k.IDVersionKeyMtx.Unlock()

		k.UpdateKeyFromSvr(userID, "", operationID)

		k.IDVersionKeyMtx.Lock()
		v, ok = k.IDMaxVersionKey[k.genUserIDPrefixKey(userID)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			return nil, v
		}
		k.IDVersionKeyMtx.Unlock()
		return errors.New("no key "), nil
	}

	if groupID != "" {
		k.IDVersionKeyMtx.Lock()
		v, ok := k.IDMaxVersionKey[k.genGroupIDPrefixKey(groupID)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			return nil, v
		}
		k.IDVersionKeyMtx.Unlock()

		k.UpdateKeyFromSvr("", groupID, operationID)

		k.IDVersionKeyMtx.Lock()
		v, ok = k.IDMaxVersionKey[k.genGroupIDPrefixKey(groupID)]
		if ok {
			k.IDVersionKeyMtx.Unlock()
			return nil, v
		}
		k.IDVersionKeyMtx.Unlock()
		return errors.New("no key "), nil
	}
	return errors.New("args failed "), nil
}
