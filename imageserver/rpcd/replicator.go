package rpcd

import (
	"encoding/gob"
	"errors"
	imgclient "github.com/Symantec/Dominator/imageserver/client"
	"github.com/Symantec/Dominator/lib/filesystem"
	"github.com/Symantec/Dominator/lib/format"
	"github.com/Symantec/Dominator/lib/hash"
	objectclient "github.com/Symantec/Dominator/lib/objectserver/client"
	"github.com/Symantec/Dominator/lib/srpc"
	"github.com/Symantec/Dominator/proto/imageserver"
	"io"
	"strings"
	"time"
)

func (t *srpcType) replicator() {
	initialTimeout := time.Second * 15
	timeout := initialTimeout
	var nextSleepStopTime time.Time
	for {
		nextSleepStopTime = time.Now().Add(timeout)
		if client, err := srpc.DialHTTP("tcp", t.replicationMaster,
			timeout); err != nil {
			t.logger.Printf("Error dialling: %s %s\n", t.replicationMaster, err)
		} else {
			if conn, err := client.Call(
				"ImageServer.GetImageUpdates"); err != nil {
				t.logger.Println(err)
			} else {
				if err := t.getUpdates(conn); err != nil {
					if err == io.EOF {
						t.logger.Println(
							"Connection to image replicator closed")
						if nextSleepStopTime.Sub(time.Now()) < 1 {
							timeout = initialTimeout
						}
					} else {
						t.logger.Println(err)
					}
				}
				conn.Close()
			}
			client.Close()
		}
		time.Sleep(nextSleepStopTime.Sub(time.Now()))
		if timeout < time.Minute {
			timeout *= 2
		}
	}
}

func (t *srpcType) getUpdates(conn *srpc.Conn) error {
	t.logger.Printf("Image replicator: connected to: %s\n", t.replicationMaster)
	replicationStartTime := time.Now()
	decoder := gob.NewDecoder(conn)
	initialImages := make(map[string]struct{})
	if t.archiveMode {
		initialImages = nil
	}
	for {
		var imageUpdate imageserver.ImageUpdate
		if err := decoder.Decode(&imageUpdate); err != nil {
			if err == io.EOF {
				return err
			}
			return errors.New("decode err: " + err.Error())
		}
		switch imageUpdate.Operation {
		case imageserver.OperationAddImage:
			if imageUpdate.Name == "" {
				if initialImages != nil {
					t.deleteMissingImages(initialImages)
					initialImages = nil
				}
				t.logger.Printf("Replicated all current images in %s\n",
					format.Duration(time.Since(replicationStartTime)))
				continue
			}
			if initialImages != nil {
				initialImages[imageUpdate.Name] = struct{}{}
			}
			if err := t.addImage(imageUpdate.Name); err != nil {
				return err
			}
		case imageserver.OperationDeleteImage:
			if t.archiveMode {
				continue
			}
			t.logger.Printf("Replicator(%s): delete image\n", imageUpdate.Name)
			if err := t.imageDataBase.DeleteImage(imageUpdate.Name,
				nil); err != nil {
				return err
			}
		case imageserver.OperationMakeDirectory:
			directory := imageUpdate.Directory
			if directory == nil {
				return errors.New("nil imageUpdate.Directory")
			}
			if err := t.imageDataBase.UpdateDirectory(*directory); err != nil {
				return err
			}
		}
	}
}

func (t *srpcType) deleteMissingImages(imagesToKeep map[string]struct{}) {
	missingImages := make([]string, 0)
	for _, imageName := range t.imageDataBase.ListImages() {
		if _, ok := imagesToKeep[imageName]; !ok {
			missingImages = append(missingImages, imageName)
		}
	}
	for _, imageName := range missingImages {
		t.logger.Printf("Replicator(%s): delete missing image\n", imageName)
		if err := t.imageDataBase.DeleteImage(imageName, nil); err != nil {
			t.logger.Println(err)
		}
	}
}

func (t *srpcType) addImage(name string) error {
	timeout := time.Second * 60
	if t.imageDataBase.CheckImage(name) || t.checkImageBeingInjected(name) {
		return nil
	}
	t.logger.Printf("Replicator(%s): add image\n", name)
	client, err := srpc.DialHTTP("tcp", t.replicationMaster, timeout)
	if err != nil {
		return err
	}
	defer client.Close()
	img, err := imgclient.GetImage(client, name)
	if err != nil {
		return err
	}
	if img == nil {
		return errors.New(name + ": not found")
	}
	t.logger.Printf("Replicator(%s): downloaded image\n", name)
	if t.archiveMode && !img.ExpiresAt.IsZero() && !*archiveExpiringImages {
		t.logger.Printf(
			"Replicator(%s): ignoring expiring image in archiver mode\n",
			name)
		return nil
	}
	img.FileSystem.RebuildInodePointers()
	if err := t.getMissingObjectsRetry(img.FileSystem); err != nil {
		return err
	}
	if err := t.imageDataBase.AddImage(img, name, nil); err != nil {
		return err
	}
	t.logger.Printf("Replicator(%s): added image\n", name)
	return nil
}

func (t *srpcType) checkImageBeingInjected(name string) bool {
	t.imagesBeingInjectedLock.Lock()
	defer t.imagesBeingInjectedLock.Unlock()
	_, ok := t.imagesBeingInjected[name]
	return ok
}

func (t *srpcType) getMissingObjectsRetry(fs *filesystem.FileSystem) error {
	err := t.getMissingObjects(fs)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		return err
	}
	t.logger.Println(err)
	if !t.deleteUnreferencedObjects(fs, false) {
		return err
	}
	t.logger.Println(
		"Replicator: retrying after deleting 10% of unreferenced objects")
	err = t.getMissingObjects(fs)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		return err
	}
	if !t.deleteUnreferencedObjects(fs, true) {
		return err
	}
	t.logger.Println(
		"Replicator: retrying after deleting remaining unreferenced objects")
	return t.getMissingObjects(fs)
}

func (t *srpcType) getMissingObjects(fs *filesystem.FileSystem) error {
	hashes := make([]hash.Hash, 0, fs.NumRegularInodes)
	for _, inode := range fs.InodeTable {
		if inode, ok := inode.(*filesystem.RegularInode); ok {
			if inode.Size > 0 {
				hashes = append(hashes, inode.Hash)
			}
		}
	}
	objectSizes, err := t.objSrv.CheckObjects(hashes)
	if err != nil {
		return err
	}
	missingObjects := make([]hash.Hash, 0)
	for index, size := range objectSizes {
		if size < 1 {
			missingObjects = append(missingObjects, hashes[index])
		}
	}
	if len(missingObjects) < 1 {
		return nil
	}
	t.logger.Printf("Replicator: downloading %d of %d objects\n",
		len(missingObjects), len(hashes))
	startTime := time.Now()
	objClient := objectclient.NewObjectClient(t.replicationMaster)
	defer objClient.Close()
	objectsReader, err := objClient.GetObjects(missingObjects)
	if err != nil {
		return err
	}
	defer objectsReader.Close()
	var totalBytes uint64
	for _, hash := range missingObjects {
		length, reader, err := objectsReader.NextObject()
		if err != nil {
			return err
		}
		_, _, err = t.objSrv.AddObject(reader, length, &hash)
		reader.Close()
		if err != nil {
			return err
		}
		totalBytes += length
	}
	timeTaken := time.Since(startTime)
	t.logger.Printf("Replicator: downloaded %d objects, %s in %s (%s/s)\n",
		len(missingObjects), format.FormatBytes(totalBytes), timeTaken,
		format.FormatBytes(uint64(float64(totalBytes)/timeTaken.Seconds())))
	return nil
}

func (t *srpcType) deleteUnreferencedObjects(fs *filesystem.FileSystem,
	all bool) bool {
	objectsMap := t.imageDataBase.ListUnreferencedObjects()
	for _, inode := range fs.InodeTable {
		if inode, ok := inode.(*filesystem.RegularInode); ok {
			delete(objectsMap, inode.Hash)
		}
	}
	numToDelete := len(objectsMap)
	if !all {
		numToDelete = numToDelete / 10
		if numToDelete < 1 {
			numToDelete = numToDelete
		}
	}
	if numToDelete < 1 {
		return false
	}
	count := 0
	var unreferencedBytes uint64
	for hashVal, size := range objectsMap {
		if err := t.objSrv.DeleteObject(hashVal); err != nil {
			t.logger.Printf("Error deleting unreferenced object: %x\n", hashVal)
			return false
		}
		unreferencedBytes += size
		count++
		if count >= numToDelete {
			break
		}
	}
	t.logger.Printf("Deleted %d unreferenced objects consuming %s\n",
		numToDelete, format.FormatBytes(unreferencedBytes))
	return true
}
