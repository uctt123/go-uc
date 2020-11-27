















package build

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/Azure/azure-storage-blob-go/azblob"
)




type AzureBlobstoreConfig struct {
	Account   string 
	Token     string 
	Container string 
}






func AzureBlobstoreUpload(path string, name string, config AzureBlobstoreConfig) error {
	if *DryRunFlag {
		fmt.Printf("would upload %q to %s/%s/%s\n", path, config.Account, config.Container, name)
		return nil
	}
	
	credential, err := azblob.NewSharedKeyCredential(config.Account, config.Token)
	if err != nil {
		return err
	}

	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	u, _ := url.Parse(fmt.Sprintf("https:
	service := azblob.NewServiceURL(*u, pipeline)

	container := service.NewContainerURL(config.Container)
	blockblob := container.NewBlockBlobURL(name)

	
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = blockblob.Upload(context.Background(), in, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{})
	return err
}


func AzureBlobstoreList(config AzureBlobstoreConfig) ([]azblob.BlobItem, error) {
	credential := azblob.NewAnonymousCredential()
	if len(config.Token) > 0 {
		c, err := azblob.NewSharedKeyCredential(config.Account, config.Token)
		if err != nil {
			return nil, err
		}
		credential = c
	}
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	u, _ := url.Parse(fmt.Sprintf("https:
	service := azblob.NewServiceURL(*u, pipeline)

	var allBlobs []azblob.BlobItem
	
	container := service.NewContainerURL(config.Container)
	nextMarker := azblob.Marker{}
	for nextMarker.NotDone() {
		res, err := container.ListBlobsFlatSegment(context.Background(), nextMarker, azblob.ListBlobsSegmentOptions{
			MaxResults: 5000, 
		})
		if err != nil {
			return nil, err
		}
		allBlobs = append(allBlobs, res.Segment.BlobItems...)
		nextMarker = res.NextMarker

	}
	return allBlobs, nil
}



func AzureBlobstoreDelete(config AzureBlobstoreConfig, blobs []azblob.BlobItem) error {
	if *DryRunFlag {
		for _, blob := range blobs {
			fmt.Printf("would delete %s (%s) from %s/%s\n", blob.Name, blob.Properties.LastModified, config.Account, config.Container)
		}
		return nil
	}
	
	credential, err := azblob.NewSharedKeyCredential(config.Account, config.Token)
	if err != nil {
		return err
	}

	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	u, _ := url.Parse(fmt.Sprintf("https:
	service := azblob.NewServiceURL(*u, pipeline)

	container := service.NewContainerURL(config.Container)

	
	for _, blob := range blobs {
		blockblob := container.NewBlockBlobURL(blob.Name)
		if _, err := blockblob.Delete(context.Background(), azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{}); err != nil {
			return err
		}
		fmt.Printf("deleted  %s (%s)\n", blob.Name, blob.Properties.LastModified)
	}
	return nil
}
