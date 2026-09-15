import { describe, expect, it, beforeEach, vi } from "vitest";

const apiMock = vi.hoisted(() => vi.fn());
vi.mock("./api.js", () => ({ api: apiMock }));

import {
  clearQueue,
  enqueueItem,
  enqueuePrivateItem,
  pendingForList,
  pendingItems,
  syncPendingItems,
} from "./offlineQueue.js";

beforeEach(() => {
  localStorage.clear();
  apiMock.mockReset();
});

describe("offlineQueue", () => {
  it("enqueues own-list items separately from private ideas", () => {
    enqueueItem("Bike", "https://x.co", "red");
    enqueuePrivateItem("u7", "Skateboard", "", "in private for dave");

    expect(pendingForList("own")).toHaveLength(1);
    expect(pendingForList("private", "u7")).toHaveLength(1);
    expect(pendingForList("private", "u9")).toHaveLength(0);
    expect(pendingItems()).toHaveLength(2);
  });

  it("syncs own items to /api/my/items and private items to the owner's list", async () => {
    enqueueItem("Bike", "https://x.co", "red");
    enqueuePrivateItem("u7", "Skateboard", "https://y.co", "");
    apiMock.mockResolvedValue({});

    const remaining = await syncPendingItems();

    expect(remaining).toBe(0);
    expect(apiMock).toHaveBeenCalledTimes(2);
    expect(apiMock).toHaveBeenCalledWith("/api/my/items", {
      method: "POST",
      body: { title: "Bike", url: "https://x.co", notes: "red" },
    });
    expect(apiMock).toHaveBeenCalledWith("/api/users/u7/private-items", {
      method: "POST",
      body: { title: "Skateboard", url: "https://y.co", notes: "" },
    });
  });

  it("drops items the server rejects permanently but keeps network failures", async () => {
    enqueueItem("Keep me", "", "");
    enqueueItem("Reject me", "", "");
    apiMock
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(Object.assign(new Error("bad"), { status: 400 }));

    const remaining = await syncPendingItems();

    expect(remaining).toBe(1);
    expect(pendingItems().map((i) => i.title)).toEqual(["Keep me"]);

    const left = await syncPendingItems();
    expect(left).toBe(0);
    expect(pendingItems()).toEqual([]);
  });

  it("clearQueue empties everything queued", () => {
    enqueueItem("A", "", "");
    enqueuePrivateItem("u7", "B", "", "");
    clearQueue();
    expect(pendingItems()).toEqual([]);
  });
});
