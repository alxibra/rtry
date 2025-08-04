# rtry

**rtry** is a lightweight Go library for handling RabbitMQ retry logic using a dedicated retry queue, exponential backoff with jitter, and configurable max-attempt limits. It supports clean separation of retry queue declarations and re-publishing logic when processing fails.

---

### ✨ Features

-  **Exponential backoff with jitter** (customizable)
-  **Retry metadata** via headers (`x-retry-count`)
-  **Max retry attempts** with graceful drop logging
-  **Inject custom backoff strategy**
-  **Clean RabbitMQ queue setup** (main + retry + bindings)

---

### 📦 Installation

```bash
go get github.com/alxibra/rtry@latest
```
