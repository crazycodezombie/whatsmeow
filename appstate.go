// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	fetchAppStateRetries               = 5
	fetchAppStateSecondsBetweenRetries = 30
)

// FetchAppState fetches updates to the given type of app state. If fullSync is true, the current
// cached state will be removed and all app state patches will be re-fetched from the server.
func (cli *Client) FetchAppState(name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	cli.appStateSyncLock.Lock()
	cli.Log.Infof("starting fetch %v app state", name)
	defer func() { cli.Log.Infof("done fetch %v app state", name) }()
	defer cli.appStateSyncLock.Unlock()
	return cli.fetchAppStateNoLock(name, fullSync, onlyIfNotSynced)
}

func (cli *Client) fetchAppStateNoLock(name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	var err error
	for i := 0; i < fetchAppStateRetries; i++ {
		err = cli.actualFetchAppStateNoLock(name, fullSync, onlyIfNotSynced)
		if err == nil {
			return nil
		}

		cli.Log.Warnf("error fetching %v appstate in try number %v: %v", name, i, err)
		if i+1 < fetchAppStateRetries {
			cli.Log.Infof("sleeping %v seconds and retry fetching %v appstate", fetchAppStateSecondsBetweenRetries, name)
			time.Sleep(fetchAppStateSecondsBetweenRetries * time.Second)
		}
	}

	return err
}

func (cli *Client) actualFetchAppStateNoLock(name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	if fullSync {
		err := cli.Store.AppState.DeleteAppStateVersion(string(name))
		if err != nil {
			return fmt.Errorf("failed to reset app state %s version: %w", name, err)
		}
	}
	version, hash, err := cli.Store.AppState.GetAppStateVersion(string(name))
	if err != nil {
		return fmt.Errorf("failed to get app state %s version: %w", name, err)
	}
	if version == 0 {
		fullSync = true
	} else if onlyIfNotSynced {
		return nil
	}

	state := appstate.HashState{Version: version, Hash: hash}

	hasMore := true
	wantSnapshot := fullSync
	archivesMap := make(map[types.JID]store.ArchivedEntry)
	contactsMap := make(map[types.JID]store.ContactEntry)
	for hasMore {
		patches, err := cli.fetchAppStatePatches(name, state.Version, wantSnapshot)
		wantSnapshot = false
		if err != nil {
			return fmt.Errorf("failed to fetch app state %s patches: %w", name, err)
		}
		hasMore = patches.HasMorePatches

		mutations, newState, err := cli.appStateProc.DecodePatches(patches, state, cli.validateMACs)
		if err != nil {
			if errors.Is(err, appstate.ErrKeyNotFound) {
				go cli.requestMissingAppStateKeys(context.TODO(), patches)
			}
			return fmt.Errorf("failed to decode app state %s patches: %w", name, err)
		}
		state = newState

		if name == appstate.WAPatchCriticalUnblockLow {
			cli.filterContacts(mutations, contactsMap)
		}

		if name == appstate.WAPatchRegularLow {
			cli.getArchivesInfo(mutations, archivesMap)
		}

		for _, mutation := range mutations {
			cli.dispatchAppState(mutation, fullSync, cli.EmitAppStateEventsOnFullSync)
		}
	}

	if len(contactsMap) > 0 {
		contacts := make([]store.ContactEntry, 0, len(contactsMap))
		for _, contact := range contactsMap {
			contacts = append(contacts, contact)
		}

		err = cli.Store.Contacts.PutAllContactNames(contacts)
		if err != nil {
			// This is a fairly serious failure, so just abort the whole thing
			return fmt.Errorf("failed to update contact store: %v", err)
		}
	}

	if len(archivesMap) > 0 {
		archives := make([]store.ArchivedEntry, 0, len(archivesMap))
		for _, archive := range archivesMap {
			archives = append(archives, archive)
		}

		cli.Log.Infof("Mass inserting app state snapshot with %d archives into the store", len(archives))
		err = cli.Store.ChatSettings.PutAllArchives(archives)
		if err != nil {
			return fmt.Errorf("failed to update archive store with data from snapshot: %v", err)
		}
	}

	if fullSync {
		cli.Log.Debugf("Full sync of app state %s completed. Current version: %d", name, state.Version)
		cli.dispatchEvent(&events.AppStateSyncComplete{Name: name})
	} else {
		cli.Log.Debugf("Synced app state %s from version %d to %d", name, version, state.Version)
	}
	return nil
}

func (cli *Client) filterContacts(mutations []appstate.Mutation, out map[types.JID]store.ContactEntry) {
	for _, mutation := range mutations {
		if mutation.Index[0] == "contact" && len(mutation.Index) > 1 {
			jid, _ := types.ParseJID(mutation.Index[1])

			switch mutation.Operation {
			case waProto.SyncdMutation_SET:
				act := mutation.Action.GetContactAction()
				out[jid] = store.ContactEntry{
					JID:       jid,
					FirstName: act.GetFirstName(),
					FullName:  act.GetFullName(),
					Exists:    true,
				}
			case waProto.SyncdMutation_REMOVE:
				out[jid] = store.ContactEntry{
					JID:    jid,
					Exists: false,
				}
			}
		}
	}
}

func (cli *Client) getArchivesInfo(mutations []appstate.Mutation, out map[types.JID]store.ArchivedEntry) {
	for _, mutation := range mutations {
		if mutation.Operation != waProto.SyncdMutation_SET {
			continue
		}

		if mutation.Index[0] != appstate.IndexArchive || len(mutation.Index) < 2 {
			continue
		}

		jid, _ := types.ParseJID(mutation.Index[1])
		archived := mutation.Action.GetArchiveChatAction().GetArchived()
		archivedTime := time.UnixMilli(*mutation.Action.Timestamp)
		out[jid] = store.ArchivedEntry{JID: jid, Archived: archived, ArchivedTime: archivedTime}
	}
}

func (cli *Client) dispatchAppStateSet(mutation appstate.Mutation, fullSync bool, emitOnFullSync bool) {
	dispatchEvts := !fullSync || emitOnFullSync

	if mutation.Operation != waProto.SyncdMutation_SET {
		return
	}

	if dispatchEvts {
		cli.dispatchEvent(&events.AppState{Index: mutation.Index, SyncActionValue: mutation.Action})
	}

	var jid types.JID
	if len(mutation.Index) > 1 {
		jid, _ = types.ParseJID(mutation.Index[1])
	}
	ts := time.UnixMilli(mutation.Action.GetTimestamp())

	var storeUpdateError error
	var eventToDispatch interface{}
	switch mutation.Index[0] {
	case appstate.IndexMute:
		act := mutation.Action.GetMuteAction()
		eventToDispatch = &events.Mute{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		var mutedUntil time.Time
		if act.GetMuted() {
			mutedUntil = time.UnixMilli(act.GetMuteEndTimestamp())
		}
		if cli.Store.ChatSettings != nil {
			storeUpdateError = cli.Store.ChatSettings.PutMutedUntil(jid, mutedUntil)
		}
	case appstate.IndexPin:
		act := mutation.Action.GetPinAction()
		eventToDispatch = &events.Pin{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if cli.Store.ChatSettings != nil {
			storeUpdateError = cli.Store.ChatSettings.PutPinned(jid, act.GetPinned())
		}
	case appstate.IndexArchive:
		act := mutation.Action.GetArchiveChatAction()
		eventToDispatch = &events.Archive{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
	case appstate.IndexContact:
		act := mutation.Action.GetContactAction()
		eventToDispatch = &events.Contact{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
	case appstate.IndexClearChat:
		act := mutation.Action.GetClearChatAction()
		eventToDispatch = &events.ClearChat{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
	case appstate.IndexDeleteChat:
		act := mutation.Action.GetDeleteChatAction()
		eventToDispatch = &events.DeleteChat{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
	case appstate.IndexStar:
		if len(mutation.Index) < 5 {
			return
		}
		evt := events.Star{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetStarAction(),
			IsFromMe:     mutation.Index[3] == "1",
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != "0" {
			evt.SenderJID, _ = types.ParseJID(mutation.Index[4])
		}
		eventToDispatch = &evt
	case appstate.IndexDeleteMessageForMe:
		if len(mutation.Index) < 5 {
			return
		}
		evt := events.DeleteForMe{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetDeleteMessageForMeAction(),
			IsFromMe:     mutation.Index[3] == "1",
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != "0" {
			evt.SenderJID, _ = types.ParseJID(mutation.Index[4])
		}
		eventToDispatch = &evt
	case appstate.IndexMarkChatAsRead:
		eventToDispatch = &events.MarkChatAsRead{
			JID:          jid,
			Timestamp:    ts,
			Action:       mutation.Action.GetMarkChatAsReadAction(),
			FromFullSync: fullSync,
		}
	case appstate.IndexSettingPushName:
		eventToDispatch = &events.PushNameSetting{
			Timestamp:    ts,
			Action:       mutation.Action.GetPushNameSetting(),
			FromFullSync: fullSync,
		}
		cli.Store.PushName = mutation.Action.GetPushNameSetting().GetName()
		err := cli.Store.Save()
		if err != nil {
			cli.Log.Errorf("Failed to save device store after updating push name: %v", err)
		}
	case appstate.IndexSettingUnarchiveChats:
		eventToDispatch = &events.UnarchiveChatsSetting{
			Timestamp:    ts,
			Action:       mutation.Action.GetUnarchiveChatsSetting(),
			FromFullSync: fullSync,
		}
		cli.Store.UnarchiveChatsSettings = mutation.Action.GetUnarchiveChatsSetting().GetUnarchiveChats()
		err := cli.Store.Save()
		if err != nil {
			cli.Log.Errorf("Failed to save device store after updating unarchive chat settings: %v", err)
		}
	case appstate.IndexUserStatusMute:
		eventToDispatch = &events.UserStatusMute{
			JID:          jid,
			Timestamp:    ts,
			Action:       mutation.Action.GetUserStatusMuteAction(),
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelEdit:
		act := mutation.Action.GetLabelEditAction()
		eventToDispatch = &events.LabelEdit{
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			Action:       act,
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelAssociationChat:
		if len(mutation.Index) < 3 {
			return
		}
		jid, _ = types.ParseJID(mutation.Index[2])
		act := mutation.Action.GetLabelAssociationAction()
		eventToDispatch = &events.LabelAssociationChat{
			JID:          jid,
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			Action:       act,
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelAssociationMessage:
		if len(mutation.Index) < 6 {
			return
		}
		jid, _ = types.ParseJID(mutation.Index[2])
		act := mutation.Action.GetLabelAssociationAction()
		eventToDispatch = &events.LabelAssociationMessage{
			JID:          jid,
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			MessageID:    mutation.Index[3],
			Action:       act,
			FromFullSync: fullSync,
		}
	}
	if storeUpdateError != nil {
		cli.Log.Errorf("Failed to update device store after app state mutation: %v", storeUpdateError)
	}
	if dispatchEvts && eventToDispatch != nil {
		cli.dispatchEvent(eventToDispatch)
	}
}

func (cli *Client) dispatchAppStateRemove(mutation appstate.Mutation, fullSync bool, emitOnFullSync bool) {
	var jid types.JID
	if len(mutation.Index) > 1 {
		jid, _ = types.ParseJID(mutation.Index[1])
	}

	var storeDeleteError error

	switch mutation.Index[0] {
	case appstate.IndexContact:
		if cli.Store.Contacts != nil {
			storeDeleteError = cli.Store.Contacts.DeleteContactName(jid)
		}
	}

	if storeDeleteError != nil {
		cli.Log.Errorf("Failed to remove data from store after app state mutation: %v", storeDeleteError)
	}
}

func (cli *Client) dispatchAppState(mutation appstate.Mutation, fullSync bool, emitOnFullSync bool) {
	switch mutation.Operation {
	case waProto.SyncdMutation_SET:
		cli.dispatchAppStateSet(mutation, fullSync, emitOnFullSync)
	case waProto.SyncdMutation_REMOVE:
		cli.dispatchAppStateRemove(mutation, fullSync, emitOnFullSync)
	default:
		cli.Log.Warnf("Got invalid type of mutation operation: %v", mutation.Operation)
		return
	}
}

func (cli *Client) downloadExternalAppStateBlob(ref *waProto.ExternalBlobReference) ([]byte, error) {
	return cli.Download(ref)
}

func (cli *Client) fetchAppStatePatches(name appstate.WAPatchName, fromVersion uint64, snapshot bool) (*appstate.PatchList, error) {
	attrs := waBinary.Attrs{
		"name":            string(name),
		"return_snapshot": snapshot,
	}
	if !snapshot {
		attrs["version"] = fromVersion
	}
	resp, err := cli.sendIQ(infoQuery{
		Namespace: "w:sync:app:state",
		Type:      "set",
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "sync",
			Content: []waBinary.Node{{
				Tag:   "collection",
				Attrs: attrs,
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	return appstate.ParsePatchList(resp, cli.downloadExternalAppStateBlob)
}

func (cli *Client) requestMissingAppStateKeys(ctx context.Context, patches *appstate.PatchList) {
	cli.appStateKeyRequestsLock.Lock()
	rawKeyIDs := cli.appStateProc.GetMissingKeyIDs(patches)
	filteredKeyIDs := make([][]byte, 0, len(rawKeyIDs))
	now := time.Now()
	for _, keyID := range rawKeyIDs {
		stringKeyID := hex.EncodeToString(keyID)
		lastRequestTime := cli.appStateKeyRequests[stringKeyID]
		if lastRequestTime.IsZero() || lastRequestTime.Add(24*time.Hour).Before(now) {
			cli.appStateKeyRequests[stringKeyID] = now
			filteredKeyIDs = append(filteredKeyIDs, keyID)
		}
	}
	cli.appStateKeyRequestsLock.Unlock()
	cli.requestAppStateKeys(ctx, filteredKeyIDs)
}

func (cli *Client) requestAppStateKeys(ctx context.Context, rawKeyIDs [][]byte) {
	keyIDs := make([]*waProto.AppStateSyncKeyId, len(rawKeyIDs))
	debugKeyIDs := make([]string, len(rawKeyIDs))
	for i, keyID := range rawKeyIDs {
		keyIDs[i] = &waProto.AppStateSyncKeyId{KeyId: keyID}
		debugKeyIDs[i] = hex.EncodeToString(keyID)
	}
	msg := &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: waProto.ProtocolMessage_APP_STATE_SYNC_KEY_REQUEST.Enum(),
			AppStateSyncKeyRequest: &waProto.AppStateSyncKeyRequest{
				KeyIds: keyIDs,
			},
		},
	}
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() || len(debugKeyIDs) == 0 {
		return
	}
	cli.Log.Infof("Sending key request for app state keys %+v", debugKeyIDs)
	_, err := cli.SendMessage(ctx, ownID, msg, SendRequestExtra{Peer: true})
	if err != nil {
		cli.Log.Warnf("Failed to send app state key request: %v", err)
	}
}

func (cli *Client) actualSendAppState(patch appstate.PatchInfo) error {
	version, hash, err := cli.Store.AppState.GetAppStateVersion(string(patch.Type))
	if err != nil {
		return err
	}
	cli.Log.Infof("%v version is %v", patch.Type, version)

	// TODO create new key instead of reusing the primary client's keys
	latestKeyID, err := cli.Store.AppStateKeys.GetLatestAppStateSyncKeyID()
	if err != nil {
		return fmt.Errorf("failed to get latest app state key ID: %w", err)
	} else if latestKeyID == nil {
		return fmt.Errorf("no app state keys found, creating app state keys is not yet supported")
	}

	state := appstate.HashState{Version: version, Hash: hash}

	encodedPatch, err := cli.appStateProc.EncodePatch(latestKeyID, state, patch)
	if err != nil {
		return err
	}

	resp, err := cli.sendIQ(infoQuery{
		Namespace: "w:sync:app:state",
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "sync",
			Content: []waBinary.Node{{
				Tag: "collection",
				Attrs: waBinary.Attrs{
					"name":            string(patch.Type),
					"version":         version,
					"return_snapshot": false,
				},
				Content: []waBinary.Node{{
					Tag:     "patch",
					Content: encodedPatch,
				}},
			}},
		}},
	})
	if err != nil {
		return err
	}

	respCollection := resp.GetChildByTag("sync", "collection")
	respCollectionAttr := respCollection.AttrGetter()
	if respCollectionAttr.OptionalString("type") == "error" {
		// TODO parse error properly
		return fmt.Errorf("%w: %s", ErrAppStateUpdate, respCollection.XMLString())
	}

	return cli.fetchAppStateNoLock(patch.Type, false, false)
}

// SendAppState sends the given app state patch, then resyncs that app state type from the server
// to update local caches and send events for the updates.
//
// You can use the Build methods in the appstate package to build the parameter for this method, e.g.
//
//	cli.SendAppState(appstate.BuildMute(targetJID, true, 24 * time.Hour))
func (cli *Client) SendAppState(patch appstate.PatchInfo) error {
	cli.appStateSyncLock.Lock()
	cli.Log.Infof("starting setting %v app state", patch.Type)
	defer func() { cli.Log.Infof("done setting %v app state", patch.Type) }()
	defer cli.appStateSyncLock.Unlock()

	err := cli.actualSendAppState(patch)
	if err == nil {
		return nil
	}

	err = cli.fetchAppStateNoLock(patch.Type, false, false)
	if err != nil {
		return err
	}

	return cli.actualSendAppState(patch)
}
