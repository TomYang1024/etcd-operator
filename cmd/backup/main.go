package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"time"

	"etcd-operator/pkg/file"

	"github.com/go-logr/zapr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/snapshot"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:scheme
)

func main() {
	var (
		etcdURL            string
		backupURL          string
		dialTimeoutSeconds int64
		timeoutSeconds     int64
	)
	flag.StringVar(&etcdURL, "etcd-url", etcdURL, "The URL of the etcd cluster")
	flag.StringVar(&backupURL, "backup-url", "", "The temporary directory to store the backup")
	flag.Int64Var(&dialTimeoutSeconds, "dial-timeout-seconds", 5, "The dial timeout in seconds")
	flag.Int64Var(&timeoutSeconds, "timeout-seconds", 30, "The timeout in seconds")
	flag.Parse()

	uri, _ := file.ParseBackupURL(backupURL)

	localPath := filepath.Join(os.TempDir(), uri.Path)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	zaplogger := zap.NewRaw(zap.UseDevMode(true))

	ctrl.SetLogger(zapr.NewLogger(zaplogger))

	if err := snapshot.Save(
		ctx,
		zap.NewRaw(zap.UseDevMode(true)),
		clientv3.Config{
			Endpoints: []string{etcdURL},
		},
		localPath,
	); err != nil {
		log.Fatalln(err)
	}

	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKeyID := os.Getenv("MINIO_ACCESS_ID")
	secretAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	useSSL := true
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln(err)
	}

	bucketName := uri.Host
	location := "us-east-1"

	err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: location})
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, errBucketExists := minioClient.BucketExists(ctx, bucketName)
		if errBucketExists == nil && exists {
			log.Printf("We already own %s\n", bucketName)
		} else {
			log.Fatalln(err)
		}
	} else {
		log.Printf("Successfully created %s\n", bucketName)
	}
	objectName := uri.Path
	contentType := "application/octet-stream"
	info, err := minioClient.FPutObject(ctx, bucketName, objectName, localPath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Successfully uploaded %s of size %d\n", objectName, info.Size)
}
