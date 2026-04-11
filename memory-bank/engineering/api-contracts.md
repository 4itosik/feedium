# API Contracts

## Proto-файлы

- Расположение: `api/feedium/`
- Один `.proto` файл на один домен (`source.proto`, `post.proto`, `feed.proto`)
- Package: `feedium`
- Go package: `feedium/api/feedium;feedium`

## Генерация кода

- protoc с kratos-плагинами
- Плагины: `protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-go-http` (kratos)
- Makefile target: `make proto`
- Сгенерированный код коммитится

### Makefile target

```makefile
.PHONY: proto
proto:
	protoc --proto_path=./api \
		--proto_path=./third_party \
		--go_out=paths=source_relative:./api \
		--go-http_out=paths=source_relative:./api \
		--go-grpc_out=paths=source_relative:./api \
		api/feedium/*.proto
```

## Транспорт

- Kratos HTTP — основной транспорт для React SPA (JSON)
- Kratos gRPC — опционально, для межсервисного взаимодействия в будущем
- Оба транспорта генерируются из одних proto-файлов
- HTTP transcoding: через kratos google.api.http annotations

### HTTP annotations в proto

```protobuf
import "google/api/annotations.proto";

service FeedService {
  rpc V1ListFeed(V1ListFeedRequest) returns (V1ListFeedResponse) {
    option (google.api.http) = {
      get: "/v1/feed"
    };
  }
}
```

## Версионирование

- Версия в имени метода: `V1ListPosts`, `V2ListPosts` — не в пакете и не в директории
- Один пакет `feedium`, один модуль — новая версия метода не требует нового модуля
- Версия в HTTP-пути задаётся явно через аннотации: `/v1/posts`, `/v2/posts`
- Request и Response тоже с префиксом версии: `V1ListPostsRequest`, `V2ListPostsRequest`
- Обратная совместимость внутри версии метода: добавлять поля можно, удалять/менять нельзя
- Старые версии методов живут рядом с новыми, пока есть хотя бы один потребитель

```protobuf
service PostService {
  // V1 — базовый список
  rpc V1ListPosts(V1ListPostsRequest) returns (V1ListPostsResponse) {
    option (google.api.http) = {
      get: "/v1/posts"
    };
  }

  // V2 — добавлена фильтрация по источнику
  rpc V2ListPosts(V2ListPostsRequest) returns (V2ListPostsResponse) {
    option (google.api.http) = {
      get: "/v2/posts"
    };
  }
}
```

## Правила proto-файлов

- Стиль: [Google Proto Style Guide](https://protobuf.dev/programming-guides/style/)
- Имена сообщений: PascalCase с V-префиксом (`V1ListFeedRequest`, `V1ListFeedResponse`)
- Имена полей: snake_case (`source_id`, `published_at`)
- Enum значения: UPPER_SNAKE_CASE с префиксом (`SOURCE_TYPE_TELEGRAM`)
- Суффикс Response (не Reply) — отклонение от конвенции kratos в пользу общепринятого стандарта
- Пустые запросы: `google.protobuf.Empty` не используем, всегда свой message

## Структура типичного proto

```protobuf
syntax = "proto3";

package feedium;

option go_package = "feedium/api/feedium;feedium";

import "google/api/annotations.proto";

service PostService {
  rpc V1ListPosts(V1ListPostsRequest) returns (V1ListPostsResponse) {
    option (google.api.http) = {
      get: "/v1/posts"
    };
  };

  rpc V1GetPost(V1GetPostRequest) returns (V1GetPostResponse) {
    option (google.api.http) = {
      get: "/v1/posts/{id}"
    };
  };
}

message V1ListPostsRequest {
  int64 page = 1;
  int64 page_size = 2;
}

message V1ListPostsResponse {
  repeated PostItem items = 1;
  int64 total = 2;
}

// Shared messages — без V-префикса, переиспользуются между версиями
message PostItem {
  int64 id = 1;
  string title = 2;
  string text = 3;
  string author = 4;
  string published_at = 5;
}
```

## Ошибки API

- Kratos status errors
- Стандартные gRPC-коды: NOT_FOUND, INVALID_ARGUMENT, INTERNAL
- Определяем ошибки в proto через `errors` (kratos proto-gen-errors)

```protobuf
// api/feedium/error_reason.proto
enum ErrorReason {
  UNKNOWN = 0;
  POST_NOT_FOUND = 1;
  SOURCE_NOT_FOUND = 2;
}
```

## third_party/

- Google API annotations: `google/api/annotations.proto`, `google/api/http.proto`
- Kratos errors: `errors/errors.proto`
- Коммитятся в репозиторий
