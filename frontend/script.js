
const MAX_TOKENS = 5;
const BURST_SIZE = 10;

const usersContainer = document.getElementById("usersContainer");
const userCountInput = document.getElementById("userCount");
const generateBtn = document.getElementById("generateBtn");
const requestLogs = document.getElementById("requestLogs");
const clearLogsBtn = document.getElementById("clearLogsBtn");
const activeUserCount = document.getElementById("activeUserCount");

let totalRequests = 0;
let allowedRequests = 0;
let deniedRequests = 0;
let failedRequests = 0;
let generation = 0;


// Update overall statistics and chart.
function updateStats() {
    document.getElementById("totalRequests").textContent =
        totalRequests;

    document.getElementById("allowedRequests").textContent =
        allowedRequests;

    document.getElementById("deniedRequests").textContent =
        deniedRequests;

    const decisions = allowedRequests + deniedRequests;

    const allowedPercent = decisions
        ? (allowedRequests / decisions) * 100
        : 0;

    const deniedPercent = decisions
        ? (deniedRequests / decisions) * 100
        : 0;

    document.getElementById("allowedBar").style.width =
        `${allowedPercent}%`;

    document.getElementById("deniedBar").style.width =
        `${deniedPercent}%`;

    document.getElementById("allowedPercentage").textContent =
        `${allowedPercent.toFixed(1)}%`;

    document.getElementById("deniedPercentage").textContent =
        `${deniedPercent.toFixed(1)}%`;
}


// Generate the user cards.
function generateUsers() {
    const count = Number(userCountInput.value);

    if (!Number.isInteger(count) || count < 1 || count > 20) {
        alert("Enter a user count between 1 and 20.");
        return;
    }

    generation++;
    usersContainer.innerHTML = "";

    activeUserCount.textContent =
        `${count} user${count === 1 ? "" : "s"}`;

    for (let i = 1; i <= count; i++) {
        createUserCard(i);
    }
}


// Create a card for one user.
function createUserCard(number) {
    const userId = `user-${number}`;

    const card = document.createElement("div");
    card.className = "user-card";

    card.innerHTML = `
        <h3>User ${number}</h3>

        <p class="token-label">
            Last reported tokens:
            <strong class="token-count">--</strong>
        </p>

        <div class="token-bar">
            <div class="token-fill"></div>
        </div>

        <div class="user-actions">
            <button class="request-btn">
                Send Request
            </button>

            <button class="burst-btn">
                Send 10 Concurrent
            </button>
        </div>

        <div class="user-status">
            Ready to send requests
        </div>

        <div class="user-request-count">
            Attempts: <span class="attempt-count">0</span>
        </div>
    `;

    const singleButton = card.querySelector(".request-btn");
    const burstButton = card.querySelector(".burst-btn");

    singleButton.addEventListener("click", () => {
        sendRequests(userId, card, 1);
    });

    burstButton.addEventListener("click", () => {
        sendRequests(userId, card, BURST_SIZE);
    });

    usersContainer.appendChild(card);
}


// One real HTTP request through our Go gateway.
async function checkLimit(userId) {
    const response = await fetch("/api/check-limit", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            user_id: userId,
            endpoint: "/products"
        })
    });

    if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
    }

    return await response.json();
}


// Send 1 or 10 concurrent requests.
async function sendRequests(userId, card, count) {
    const currentGeneration = generation;

    const singleButton = card.querySelector(".request-btn");
    const burstButton = card.querySelector(".burst-btn");
    const statusElement = card.querySelector(".user-status");
    const attemptCount = card.querySelector(".attempt-count");

    // Prevent overlapping batches from the same card.
    singleButton.disabled = true;
    burstButton.disabled = true;

    statusElement.className = "user-status";
    statusElement.textContent =
        `Sending ${count} request${count === 1 ? "" : "s"}...`;

    try {
        // Start every HTTP request without awaiting
        // an earlier request first.
        const tasks = Array.from(
            { length: count },
            () => checkLimit(userId)
        );

        // Each task reports success or failure separately.
        const results = await Promise.allSettled(tasks);

        // User cards might have been regenerated.
        if (currentGeneration !== generation) return;

        let batchAllowed = 0;
        let batchDenied = 0;
        let batchFailed = 0;
        let latestTokens = null;
        let maxRetryMs = 0;

        for (const result of results) {
            totalRequests++;

            if (result.status === "rejected") {
                batchFailed++;
                failedRequests++;

                addRequestLog(userId, "ERROR");
                continue;
            }

            const data = result.value;

            latestTokens = data.remaining_tokens;

            if (data.allowed) {
                batchAllowed++;
                allowedRequests++;

                addRequestLog(userId, "ALLOWED");
            } else {
                batchDenied++;
                deniedRequests++;

                maxRetryMs = Math.max(
                    maxRetryMs,
                    data.retry_after_ms
                );

                addRequestLog(userId, "DENIED");
            }
        }

        const attempts = Number(attemptCount.textContent);
        attemptCount.textContent = attempts + count;

        // This is a snapshot from one completed response,
        // not a guaranteed real-time token balance.
        if (latestTokens !== null) {
            updateTokenDisplay(card, latestTokens);
        }

        updateStats();

        if (batchFailed > 0) {
            statusElement.className = "user-status error";
        } else if (batchDenied > 0) {
            statusElement.className = "user-status denied";
        } else {
            statusElement.className = "user-status allowed";
        }

        statusElement.textContent =
            `Allowed: ${batchAllowed} | ` +
            `Denied: ${batchDenied} | ` +
            `Errors: ${batchFailed}`;

        if (batchDenied > 0) {
            statusElement.textContent +=
                ` | Retry ≈ ${maxRetryMs}ms`;
        }

    } finally {
        if (currentGeneration === generation) {
            singleButton.disabled = false;
            burstButton.disabled = false;
        }
    }
}


// Update the displayed token snapshot.
function updateTokenDisplay(card, remaining) {
    card.querySelector(".token-count").textContent = remaining;

    const percent = Math.min(
        100,
        Math.max(0, (remaining / MAX_TOKENS) * 100)
    );

    card.querySelector(".token-fill").style.width =
        `${percent}%`;
}


// Display an individual request result.
function addRequestLog(userId, result) {
    const empty = requestLogs.querySelector(".empty-message");
    if (empty) empty.remove();

    const entry = document.createElement("div");
    entry.className = "log-entry";

    const info = document.createElement("div");
    info.className = "log-info";

    const name = document.createElement("strong");
    name.textContent = userId;

    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = new Date().toLocaleTimeString();

    info.append(name, time);

    const badge = document.createElement("span");
    badge.className =
        `status-badge ${result.toLowerCase()}`;
    badge.textContent = result;

    entry.append(info, badge);
    requestLogs.prepend(entry);

    while (requestLogs.children.length > 20) {
        requestLogs.lastElementChild.remove();
    }
}


// Reset only dashboard statistics.
function clearHistory() {
    totalRequests = 0;
    allowedRequests = 0;
    deniedRequests = 0;
    failedRequests = 0;

    requestLogs.innerHTML = `
        <p class="empty-message">
            No requests made yet.
        </p>
    `;

    updateStats();
}

generateBtn.addEventListener("click", generateUsers);
clearLogsBtn.addEventListener("click", clearHistory);

// Initial four users.
generateUsers();
