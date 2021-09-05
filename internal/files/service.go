package files

type Service interface{
  Create()
  CreateDir()
  Namespace(string) Service
}
