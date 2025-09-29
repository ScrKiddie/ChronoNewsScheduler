# ChronoNewsScheduler



ChronoNewsScheduler is a robust background job processor built with Go. Designed as an integral part of the **[ChronoNewsAPI](https://github.com/ScrKiddie/ChronoNewsAPI)** project, it handles resource-intensive tasks asynchronously to maintain a fast and responsive API.

## Features

- **Image Compression**: Automatically processes and compresses uploaded images to the modern WebP format.
- **Concurrent & Sequential Modes**: Choose between a high-performance concurrent pipeline for processing multiple images at once or a simpler sequential mode.
- **Cleanup Service**: A "cleanup" scheduler that automatically deletes old, unused files from storage to save space.
- **Janitor Service**: A "janitor" scheduler that finds and resets tasks that are stuck in a `processing` state for too long.
- **Robust Error Handling**: Features an automatic retry mechanism for failed tasks and a Dead Letter Queue (DLQ) for tasks that fail permanently.
- **Highly Configurable**: All functionalities are easily configured through environment variables.
- **Structured Logging**: Provides clear, structured logs for easy monitoring and debugging.

## Architecture Diagram

```mermaid
graph TD
    %% Define Core Components & Main App
    subgraph "Core Infrastructure"
        DB[(Database)]
        FS[(Filesystem)]
    end
    
    MainApp[Main ChronoNews API]

    %% Define Schedulers
    subgraph "Background Services (This Project)"
        CompScheduler(Compression Scheduler)
        JanitorScheduler(Janitor Scheduler)
        CleanupScheduler(Cleanup Scheduler)
    end

    %% Define Interactions
    MainApp -- "Updates post/file relationship" --> DB

    CompScheduler -- "1. Finds 'pending' tasks" --> DB
    CompScheduler -- "2. Reads source image" --> FS
    CompScheduler -- "3. Writes compressed image" --> FS
    CompScheduler -- "4. Updates status to 'compressed' or 'failed'" --> DB

    JanitorScheduler -- "1. Finds stuck 'processing' tasks" --> DB
    JanitorScheduler -- "2. Resets status to 'pending'" --> DB

    CleanupScheduler -- "1. Finds old & unused file records" --> DB
    CleanupScheduler -- "2. Deletes physical file" --> FS
    CleanupScheduler -- "3. Deletes file record" --> DB
```

## Dependencies

To run this project, your only required system dependency is the `libvips` library. The Go bindings included in this repository were pre-generated using the [vipsgen](https://github.com/cshum/vipsgen) tool and are specifically tailored for **`libvips` version `8.12.1`**. This ensures the project works out-of-the-box if you have the same version installed. If you need to use a different version of `libvips` or wish to customize the bindings, you can easily regenerate them yourself by running the `vipsgen` tool again from the project's root directory. Otherwise, simply install the matching `libvips` version, run `go mod tidy`, and you are ready to go.
## Configuration

All application settings are managed via an `.env` file. Create one based on `.env.example`.

| Variable                      | Description                                                                          | Example Value              |
| ----------------------------- | ------------------------------------------------------------------------------------ | -------------------------- |
| **Database & General** |                                                                                      |                            |
| `DB_HOST`                     | Database host address.                                                               | `localhost`                |
| `DB_USER`                     | Database username.                                                                   | `postgres`                 |
| `DB_PASSWORD`                 | Database password.                                                                   | `secret`                   |
| `DB_NAME`                     | Database name.                                                                       | `chrononews_db`            |
| `DB_PORT`                     | Database port.                                                                       | `5432`                     |
| `LOG_LEVEL`                   | Log level (`debug`, `info`, `warn`, `error`).                                        | `info`                     |
| **Compression Scheduler** |                                                                                      |                            |
| `COMPRESSION_SCHEDULE`        | Cron schedule for the compression job. Leave empty to disable.                       | `'*/5 * * * *'`            |
| `COMPRESSION_SOURCE_DIR`      | The directory to read original images from.                                          | `./images/source`          |
| `COMPRESSION_DEST_DIR`        | The directory to write compressed images to.                                         | `./images/compressed`      |
| `COMPRESSION_IS_TEST_MODE`    | If `true`, skips database updates for testing purposes.                                | `false`                    |
| `COMPRESSION_MAX_RETRIES`     | Max number of retries before a task is sent to the DLQ.                                | `3`                        |
| `COMPRESSION_IS_CONCURRENT`   | Use the high-performance concurrent pipeline (`true`) or sequential mode (`false`).    | `true`                     |
| `COMPRESSION_BATCH_SIZE`      | Number of images to process in one run.                                              | `50`                       |
| `COMPRESSION_NUM_IO_WORKERS`  | Number of workers for file I/O in concurrent mode.                                   | `8`                        |
| `COMPRESSION_NUM_CPU_WORKERS` | Number of workers for image processing in concurrent mode.                           | `4`                        |
| `COMPRESSION_WEBP_QUALITY`    | Compression quality for WebP images (1-100).                                         | `75`                       |
| `COMPRESSION_MAX_WIDTH`       | Maximum width for resized images.                                                    | `1920`                     |
| `COMPRESSION_MAX_HEIGHT`      | Maximum height for resized images.                                                   | `1080`                     |
| **Cleanup Scheduler** |                                                                                      |                            |
| `CLEANUP_SCHEDULE`            | Cron schedule for the cleanup job. Leave empty to disable.                           | `'0 2 * * *'`              |
| `CLEANUP_THRESHOLD`           | How old an unused file must be before it's deleted (e.g., `720h` for 30 days).         | `720h`                     |
| **Janitor Scheduler** |                                                                                      |                            |
| `JANITOR_SCHEDULE`            | Cron schedule for the janitor job. Leave empty to disable.                           | `'*/15 * * * *'`           |
| `JANITOR_STUCK_THRESHOLD`     | How long a task can be 'processing' before it's considered stuck (e.g., `30m`).        | `30m`                      |
