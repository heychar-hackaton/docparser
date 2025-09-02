# docparser

HTTP-сервис на Go для извлечения текста из файлов (pdf, docx, rtf, txt).

## Возможности
- POST `/extract` — принимает JSON `{ filename, content_base64 }`, возвращает `{ success, text }`.
- GET `/health` — статус сервиса.
- Поддерживаемые форматы: `.pdf`, `.docx`, `.rtf`, `.txt`.
- PDF обрабатывается через системный `pdftotext` (Poppler).
- DOCX распаковывается и читается напрямую из `word/document.xml`.
- RTF — упрощённый парсер с нормализацией пробелов/переносов.
- TXT — авто-детекция кодировок (Windows-1251, KOI8-R, ISO-8859-5, MacCyrillic, CP866) + нормализация переводов строк.

## Требования
- Go 1.22+
- Для PDF: установленный `pdftotext` из состава Poppler (или Xpdf).

### Быстрая установка `pdftotext`
Используйте скрипт:
```bash
scripts/install_pdftotext.sh
```
Скрипт поддерживает macOS (brew), Ubuntu/Debian (apt), Fedora (dnf), RHEL/CentOS (yum/dnf), Arch (pacman), Alpine (apk), openSUSE/SLES (zypper). Если `pdftotext` уже установлен — скрипт завершится успешно.

Альтернативно, вручную:
- macOS: `brew install poppler`
- Ubuntu/Debian: `sudo apt-get install -y poppler-utils`
- Fedora: `sudo dnf install -y poppler-utils`
- Arch: `sudo pacman -S poppler`
- Alpine: `sudo apk add poppler-utils`
- openSUSE/SLES: `sudo zypper install -y poppler-tools`

## Сборка
```bash
# из корня репо
go build ./...
```

## Запуск
Порт задаётся флагом `-port` (по умолчанию 8080):
```bash
# старт на 8080
go run ./cmd/server

# кастомный порт
go run ./cmd/server -port 9090
```

## Примеры запросов
### Health
```bash
curl -s http://localhost:8080/health
```

### Extract (TXT)
```bash
# "Hello, world!\n" в base64
curl -s -X POST http://localhost:8080/extract \
  -H 'Content-Type: application/json' \
  -d '{"filename":"sample.txt","content_base64":"SGVsbG8sIHdvcmxkIQo="}'
```
Ответ:
```json
{"success": true, "text": "Hello, world!\n"}
```

### Extract (PDF)
```bash
# Перед вызовом убедитесь, что установлен pdftotext
curl -s -X POST http://localhost:8080/extract \
  -H 'Content-Type: application/json' \
  -d '{"filename":"doc.pdf","content_base64":"<BASE64_OF_PDF>"}'
```
Если `pdftotext` не установлен, сервис вернёт:
```json
{"success": false, "text": "exec: \"pdftotext\": executable file not found in $PATH"}
```

### Extract (Batch)
```bash
curl -s -X POST http://localhost:8080/extract/batch \
  -H 'Content-Type: application/json' \
  -d '{
        "files": [
          {"filename": "a.txt", "content_base64": "SGVsbG8h"},
          {"filename": "b.rtf", "content_base64": "{\\rtf1...}"}
        ]
      }'
```
Ответ:
```json
{
  "results": [
    {"filename":"a.txt","success":true,"text":"Hello!"},
    {"filename":"b.rtf","success":true,"text":"..."}
  ]
}
```

## Формат ответа
- Успех: `{ "success": true, "text": "...извлечённый текст..." }`
- Ошибка: `{ "success": false, "text": "описание ошибки" }`

## Примечания
- RTF-парсер реализован упрощённо (поддержка `\par`, `\line`, `\tab`, `\uN`, `\'hh` и игнор некоторых destination-групп). Для нетипичных RTF возможны артефакты; присылайте образцы для улучшений.
- TXT-детектор кодировки использует эвристику: выбирается лучшая из популярных кириллических кодировок, далее нормализация CRLF/CR→LF.


