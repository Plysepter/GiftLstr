import { readable } from "svelte/store";

function parseHash(): string {
  return window.location.hash.replace(/^#/, "") || "/";
}

export const route = readable<string>(parseHash(), (set) => {
  const onHash = (): void => set(parseHash());
  window.addEventListener("hashchange", onHash);
  return () => window.removeEventListener("hashchange", onHash);
});

const USER_LIST = /^\/list\/([^/]+)$/;
const ARCHIVE_DETAIL = /^\/archive\/([^/]+)$/;

export interface ParsedRoute {
  name:
    | "myList"
    | "userList"
    | "lists"
    | "archive"
    | "archiveDetail"
    | "profile"
    | "admin"
    | "notFound";
  id?: string;
}

export function parseRoute(path: string): ParsedRoute {
  if (path === "/" || path === "") return { name: "myList" };
  const userList = USER_LIST.exec(path);
  if (userList) return { name: "userList", id: userList[1]! };
  switch (path) {
    case "/lists":
      return { name: "lists" };
    case "/archive":
      return { name: "archive" };
    case "/profile":
      return { name: "profile" };
    case "/admin":
      return { name: "admin" };
  }
  const archiveDetail = ARCHIVE_DETAIL.exec(path);
  if (archiveDetail) return { name: "archiveDetail", id: archiveDetail[1]! };
  return { name: "notFound" };
}
