package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var comprehendClient *comprehend.Client
var dynamodbClient *dynamodb.Client
var s3Client *s3.Client

var table string

func init() {
	table = os.Getenv("TABLE_NAME")

	if table == "" {
		log.Fatal("missing environment variable TABLE_NAME")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		log.Fatal("failed to load config ", err)
	}

	comprehendClient = comprehend.NewFromConfig(cfg)
	dynamodbClient = dynamodb.NewFromConfig(cfg)
	s3Client = s3.NewFromConfig(cfg)

}

func handler(ctx context.Context, s3Event events.S3Event) {
	for _, record := range s3Event.Records {

		fmt.Println("file", record.S3.Object.Key, "uploaded to", record.S3.Bucket.Name)

		sourceBucketName := record.S3.Bucket.Name
		fileName := record.S3.Object.Key

		err := detectEntities(sourceBucketName, fileName)

		if err != nil {
			log.Fatal("failed to process file ", record.S3.Object.Key, " in bucket ", record.S3.Bucket.Name, err)
		}
	}
}

func main() {
	lambda.Start(handler)
}

func detectEntities(sourceBucketName, fileName string) error {

	result, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(sourceBucketName),
		Key:    aws.String(fileName),
	})

	if err != nil {
		return err
	}

	fmt.Println("successfully read file", fileName, "from s3 bucket", sourceBucketName)

	buffer := new(bytes.Buffer)
	buffer.ReadFrom(result.Body)
	text := buffer.String()

	resp, err := comprehendClient.DetectEntities(context.Background(), &comprehend.DetectEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCodeEn,
	})

	if err != nil {
		return err
	}

	for _, entity := range resp.Entities {

		item := make(map[string]ddbTypes.AttributeValue)

		fmt.Printf("Type: %v\n", entity.Type)
		fmt.Printf("Text: %v\n", *entity.Text)
		fmt.Printf("Score: %v\n", *entity.Score)
		fmt.Println()

		//item["source_file"] = &ddbTypes.AttributeValueMemberS{Value: fileName}
		item["entity_type"] = &ddbTypes.AttributeValueMemberS{Value: fmt.Sprintf("%s#%v", fileName, entity.Type)}
		item["entity_name"] = &ddbTypes.AttributeValueMemberS{Value: *entity.Text}
		item["confidence_score"] = &ddbTypes.AttributeValueMemberS{Value: fmt.Sprintf("%v", *entity.Score)}

		_, err := dynamodbClient.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(table),
			Item:      item,
		})

		if err != nil {
			return err
		}

		fmt.Println("entity details added to table")

	}

	return nil
}
