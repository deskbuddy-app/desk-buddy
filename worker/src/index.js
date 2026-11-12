// Desk Buddy backend on Cloudflare Workers + D1.
// Same API as backend/server.py, so the Kindle app, phone page, helpers and Claude hook
// only need the new address. Every request needs the key (?key=, X-Key header, or the
// cookie the phone gets after opening the link with the key once).

import { loadCalendars, stale } from "./calendar.js";
import { weather, findPlace, findPlaces } from "./weather.js";
import release from "./release.json";
import { DEMO_TASKS, demoEvents } from "./demo.js";

const PROFILES = ["work", "home"];
// Task lists: one per computer ("work" is the Mac's list, "home" the Windows PC's; the old
// names stay as storage keys), or one list for everything (kept under "work").
const MACHINES = { mac: ["work", "Mac"], pc: ["home", "Windows"] };
const COOKIE = "kd";

// Settings kept on the backend (they affect the computers too). Changed from the Kindle.
const SETTINGS = {
  music_sync: { def: true, ok: (v) => typeof v === "boolean" },
  album_covers: { def: true, ok: (v) => typeof v === "boolean" },
  music_app: { def: "auto", ok: (v) => ["auto", "spotify", "music"].includes(v) }, // which app the helper reads
  focus_min: { def: 25, ok: (v) => [15, 25, 45, 50, 60].includes(v) },
  break_min: { def: 5, ok: (v) => [5, 10, 15].includes(v) },
  task_lists: { def: "per_computer", ok: (v) => ["per_computer", "one"].includes(v) },
  auto_clear: { def: false, ok: (v) => typeof v === "boolean" }, // delete done tasks after ticking…
  auto_clear_min: { def: 5, ok: (v) => [0, 1, 5, 10].includes(v) }, // …this many minutes after (0: at once)
};
const MACHINE_FOR = { work: "mac", home: "pc" };

// The Kindle's own settings (its screen, clock, heads-up, updates), kept here too so the Mac
// helper can show and change them; the Kindle syncs them both ways (newest change wins).
const KINDLE_SETTINGS = {
  theme: (v) => ["light", "dark", "auto"].includes(v),
  night_light: (v) => [-1, 0, 4, 10].includes(v),
  font: (v) => ["sans", "serif", "inter", "bitter"].includes(v),
  temp_f: (v) => typeof v === "boolean",
  refresh: (v) => [0, 1, 2].includes(v),
  heads_up: (v) => typeof v === "boolean",
  heads_up_min: (v) => [2, 5, 10].includes(v),
  clock_12h: (v) => typeof v === "boolean",
  clock_quote: (v) => typeof v === "boolean",
  clock_event: (v) => typeof v === "boolean",
  update_show: (v) => typeof v === "boolean",
  update_every: (v) => [1, 12, 24].includes(v),
  music_logo: (v) => typeof v === "boolean",
  compact: (v) => typeof v === "boolean",
  rotation: (v) => [90, 270].includes(v),
  style: (v) => ["plain", "garden", "sketch", "classic", "retro", "notebook", "typewriter", "gameboy"].includes(v), // Settings › Themes
};

// Starting deck tiles. `app` is the Mac app name (opened with `open -a`); `win` is what
// Windows opens (an app's name on its list, a link like spotify: or a program like chrome.exe).
// The PC's are replaced by PC_STARTERS when it first sends its apps. Edit with PUT /api/deck.
const DEFAULT_DECK = {
  work: ["Slack", "Figma", "Notion", "Linear", "Google Chrome", "Spotify", "Mail", "Calendar", "Claude", "Terminal", "Finder", "Notes"]
    .map((a) => ({ label: a.replace("Google ", ""), app: a })),
  home: [["Spotify", "spotify:"], ["Discord", "discord:"], ["Steam", "steam:"], ["Chrome", "chrome.exe"], ["WhatsApp", "whatsapp:"], ["Files", "explorer.exe"]]
    .map(([label, win]) => ({ label, win })),
};

// The PC's first favourites, from the apps actually on it (its first app list): these, in
// this order, up to 12. Each line is one tile: the first of its names that's on the PC.
const PC_STARTERS = [
  ["Spotify"], ["Google Chrome", "Microsoft Edge"], ["Firefox", "Mozilla Firefox"], ["Discord"], ["WhatsApp"], ["Steam"],
  ["Slack"], ["Microsoft Teams", "Microsoft Teams (work or school)", "Microsoft Teams classic"], ["Outlook (new)", "Outlook", "Microsoft Outlook"],
  ["Figma"], ["Notion"], ["File Explorer"], ["Settings"], ["Calculator"], ["Notepad"],
  ["Visual Studio Code"], ["Obsidian"], ["Zoom Workplace", "Zoom"], ["Telegram Desktop", "Telegram"], ["Epic Games Launcher"],
  ["Xbox"], ["Netflix"], ["Photos"], ["Word"], ["Excel"], ["Paint"], ["Snipping Tool"], ["Microsoft Store"],
];
const STARTER_LABELS = { "File Explorer": "Files", "Visual Studio Code": "VS Code", "Zoom Workplace": "Zoom", "Telegram Desktop": "Telegram", "Epic Games Launcher": "Epic Games" };

function starterTiles(apps) {
  const byName = new Map(apps.map((a) => [a.toLowerCase(), a]));
  const tiles = [];
  for (const names of PC_STARTERS) {
    const win = names.map((n) => byName.get(n.toLowerCase())).find(Boolean);
    if (!win) continue;
    const label = STARTER_LABELS[win] || win.replace(/^(Google|Microsoft|Mozilla) /, "").replace(/ \(.*\)$/, ""); // "Chrome", like the Mac's
    tiles.push({ label, win });
    if (tiles.length === 12) break;
  }
  return tiles;
}

export default {
  async fetch(request, env, ctx) {
    try {
      return await handle(request, env, ctx);
    } catch (e) {
      if (e instanceof HttpError) return text(e.message, e.status);
      console.error(e);
      return text("Something went wrong on the server.", 500);
    }
  },
};

class HttpError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

// ---------- routing ----------

async function handle(request, env, ctx) {
  const url = new URL(request.url);
  const path = url.pathname.replace(/\/+$/, "") || "/";
  const method = request.method;

  const given = url.searchParams.get("key") || request.headers.get("x-key") || cookie(request, COOKIE);
  // Changing the key (PUBLISHING.md): while the secret KEY_OLD is set, the old key still works,
  // the Kindle is handed the new one (desk `new_key`) and a page's cookie is renewed.
  const oldKey = !!env.KEY_OLD && !!given && given === env.KEY_OLD;
  const authed = (!!env.KEY && given === env.KEY) || oldKey;
  if (env.LOCAL && path.startsWith("/api/pair")) {
    const r = pairing(path, method, authed, env.KEY, await body(request).catch(() => ({})));
    if (r) return r;
  }
  // Connecting another computer (Desk Buddy for Windows): the Mac app makes a one-time code
  // (POST /api/connect/offer, with the key), the new computer sends it here and gets the key.
  // Valid 10 minutes, used once; 10 wrong tries end it. Works on Cloudflare and locally.
  if (path === "/api/connect" && method === "POST") {
    const store = new Store(env.DB, env.TZ || "Europe/London", ctx);
    const offer = await store.get("connect_code");
    const given = String((await body(request).catch(() => ({}))).code || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
    if (!offer || now() > offer.until) return text("That code has expired. Make a new one in Desk Buddy on your Mac.", 410);
    if (given !== offer.code) {
      const tries = (offer.tries || 0) + 1;
      await store.set("connect_code", tries >= 10 ? null : { ...offer, tries });
      return text("That code isn't right.", 403);
    }
    await store.set("connect_code", null);
    return json({ key: env.KEY });
  }
  if (!authed) {
    return html(`<!DOCTYPE html><meta name="viewport" content="width=device-width,initial-scale=1"><title>Desk Buddy</title>
<p style="font:16px system-ui;margin:2em">Open the Desk Buddy link with the key once on this device. It's remembered after that.</p>`, 401);
  }
  // Opening a page with ?key= once pairs the device: remember the key in a cookie for a year.
  const pair = (url.searchParams.get("key") || oldKey) && method === "GET" && !path.startsWith("/api/");
  const db = new Store(env.DB, env.TZ || "Europe/London", ctx);
  if (oldKey) { // who still uses the old key (to know when KEY_OLD can go)
    const noted = db.set("old_key_used", { at: now(), path, ua: (request.headers.get("user-agent") || "").slice(0, 60) });
    if (ctx?.waitUntil) ctx.waitUntil(noted);
    else await noted;
  }

  let m;
  const res = await (async () => {
    if (path === "/" && method === "GET") return redirect("/app");
    if (path === "/app" && method === "GET") return html(await phonePage(db, url.searchParams.get("list") || ""));

    // JSON API
    if (path === "/api/state" && method === "GET") return json(await db.state());
    if (path === "/api/desk" && method === "GET") {
      // ?music=1: the Kindle is showing the Music widget, so the computers should watch
      // their music app for the next minute (they stay quiet otherwise).
      if (url.searchParams.get("music") === "1" && (await db.settings()).music_sync) await db.wantMusic();
      if (url.searchParams.has("kb")) await db.kindleSeen(url.searchParams); // the Kindle says how it is
      // ?wait=25&rev=…: hold the request until something changes (any change bumps "rev"), at most
      // 25 s, so changes show at once and the Kindle asks less often. The reply always carries rev.
      const since = url.searchParams.get("rev");
      if (since && url.searchParams.has("wait")) {
        const until = Date.now() + Math.min(25, Number(url.searchParams.get("wait")) || 0) * 1000;
        while (String((await db.get("rev")) || 0) === since && Date.now() < until && !request.signal?.aborted) {
          await new Promise((r) => setTimeout(r, 1000));
        }
      }
      // move_to: the computer moved Desk Buddy to another server; the Kindle follows (main.go)
      // cloud: the cloud's address, so a Kindle on any server can always go back to it
      const cloudAt = env.LOCAL ? env.CLOUD || "" : url.origin;
      return json({ ...(await db.desk()), rev: String((await db.get("rev")) || 0), move_to: moveNote(await db.get("move_to")), cloud: cloudAt,
        ...(oldKey ? { new_key: env.KEY } : {}) });
    }
    if (path === "/api/tasks" && method === "POST") {
      const b = await body(request);
      return json({ id: await db.addTask(b.profile || "work", b.text || "", b.tag || null, b.parent_id || null, b.notes || null) });
    }
    // drag and drop: {parent_id: id | null, before: id | null}
    if ((m = path.match(/^\/api\/tasks\/(\d+)\/place$/)) && method === "POST") {
      const b = await body(request);
      await db.place(+m[1], b.parent_id ?? null, b.before ?? null);
      return json({ ok: true });
    }
    if ((m = path.match(/^\/api\/tasks\/(\d+)$/)) && method === "PATCH") {
      await db.update(+m[1], await body(request));
      return json({ ok: true });
    }
    // Everything, to move between Cloudflare and a local server (the helper's "Cloud / This Mac").
    if (path === "/api/export" && method === "GET") return json(await db.exportAll());
    if (path === "/api/import" && method === "POST") return json(await db.importAll(await body(request)));
    // where this backend runs (the Kindle and the helper show it)
    if (path === "/api/where" && method === "GET") return json({ local: !!env.LOCAL });
    // Clear a profile's list (Settings on the Kindle): everything, or only what's done.
    if (path === "/api/tasks/clear" && method === "POST") {
      const b = await body(request);
      return json({ deleted: await db.clear(b.profile, !!b.done_only) });
    }
    if ((m = path.match(/^\/api\/tasks\/(\d+)\/move\/(up|down)$/)) && method === "POST") {
      await db.move(+m[1], m[2] === "up" ? -1 : 1);
      return json({ ok: true });
    }
    if ((m = path.match(/^\/api\/tasks\/(\d+)\/(toggle|pin)$/)) && method === "POST") {
      await (m[2] === "toggle" ? db.toggle(+m[1]) : db.pin(+m[1]));
      return json({ ok: true });
    }
    if ((m = path.match(/^\/api\/tasks\/(\d+)$/)) && method === "DELETE") {
      await db.del(+m[1]);
      return json({ ok: true });
    }
    if ((m = path.match(/^\/api\/unpin\/(\w+)$/)) && method === "POST") {
      await db.unpin(m[1]);
      return json({ ok: true });
    }
    if (path === "/api/wake" && method === "POST") {
      await db.set("wake", now() + 600);
      return json({ ok: true });
    }
    // The computer moved Desk Buddy to another server (Cloud ↔ a computer): {server, name}, or
    // {server: ""} on the new one to clear an old note. The Kindle reads it from /api/desk.
    if (path === "/api/kindle/move" && method === "POST") {
      const b = await body(request);
      const server = String(b.server || "");
      if (server && !/^https?:\/\/[^\s"]+$/.test(server)) throw new HttpError(400, "server must be an http(s) address");
      await db.set("move_to", server ? { server, name: String(b.name || "").slice(0, 60), at: now() } : null);
      return json({ ok: true });
    }
    // a one-time code for connecting another computer (see /api/connect above)
    if (path === "/api/connect/offer" && method === "POST") {
      const abc = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789", r = crypto.getRandomValues(new Uint8Array(8));
      const code = Array.from(r, (n) => abc[n % abc.length]).join("");
      await db.set("connect_code", { code, until: now() + 600, tries: 0 });
      return json({ code: code.slice(0, 4) + "-" + code.slice(4), minutes: 10 });
    }
    // what Desk Buddy on a computer reports when something fails (like /api/kindle/log)
    if (path === "/api/helper/log" && method === "POST") {
      const b = await body(request);
      const log = ((await db.get("helper_log")) || []).slice(-49);
      log.push({ at: new Date().toISOString(), machine: String(b.machine || "").slice(0, 8), text: String(b.text || "").slice(0, 500) });
      await db.set("helper_log", log);
      return json({ ok: true });
    }
    if (path === "/api/helper/log" && method === "GET") return json((await db.get("helper_log")) || []);
    if (path === "/api/heartbeat" && method === "POST") {
      const b = await body(request);
      if (!(b.machine in MACHINES)) throw new HttpError(400, "machine must be mac or pc");
      const hb = (await db.get("heartbeats")) || {};
      const before = (await db.resolve())[0], wasDark = (hb[b.machine] || {}).dark, wasOS = (hb[b.machine] || {}).os;
      const os = b.machine === "pc" ? (b.os === "linux" ? "linux" : b.os === "windows" ? "windows" : wasOS) : undefined;
      hb[b.machine] = {
        at: now(),
        idle: Number(b.idle) || 0,
        ...(typeof b.dark === "boolean" ? { dark: b.dark } : {}),
        ...(os ? { os } : {}),
      };
      await db.set("heartbeats", hb);
      if (b.machine === "pc" && os) await db.set("pc_os", os);
      const profile = (await db.resolve())[0];
      if (profile !== before || (typeof b.dark === "boolean" && b.dark !== wasDark) || (os && os !== wasOS)) await db.changed();
      return json({ profile });
    }
    if (path === "/api/claude" && method === "POST") {
      const b = await body(request);
      if (!["working", "input", "done", "idle"].includes(b.state)) throw new HttpError(400, "state must be working, input, done or idle");
      await db.set("claude", { state: b.state, text: String(b.text ?? "").slice(0, 80), at: now() });
      return json({ ok: true });
    }
    if (path === "/api/profile" && method === "POST") {
      await db.setMode((await body(request)).mode);
      return json({ mode: (await db.state()).mode });
    }
    // Calendars
    if (path === "/api/calendar" && method === "GET") {
      const from = +url.searchParams.get("from") || now() - 86400, to = +url.searchParams.get("to") || now() + 30 * 86400;
      const profile = url.searchParams.get("profile");
      const c = await db.calendarCache();
      return json({
        events: c.events.filter((e) => e.end >= from && e.start < to && (!profile || e.profile === profile || e.profile === "both")),
        errors: c.errors,
      });
    }
    // Which calendars show, and on which list: {name, show: both | work | home | off}
    if (path === "/api/calendars/show" && method === "POST") {
      const b = await body(request);
      if (!b.name || !["both", "work", "home", "off"].includes(b.show)) throw new HttpError(400, "Send {name, show: both|work|home|off}.");
      const prefs = (await db.get("cal_prefs")) || {};
      if (b.show === "both") delete prefs[String(b.name)];
      else prefs[String(b.name).slice(0, 100)] = b.show;
      await db.set("cal_prefs", prefs);
      return json({ ok: true });
    }
    if (path === "/api/calendars" && method === "GET") {
      return json(((await db.get("calendars")) || []).map((c) => ({ name: c.name, profile: c.profile })));
    }
    if (path === "/api/calendars" && method === "POST") {
      await db.addCalendar(await body(request));
      return json({ ok: true });
    }
    if ((m = path.match(/^\/api\/calendars\/(\d+)$/)) && method === "DELETE") {
      await db.removeCalendar(+m[1]);
      return json({ ok: true });
    }
    if (path === "/api/calendar/push" && method === "POST") {
      // The Mac helper sends every event from the Calendar app (all accounts) here.
      const b = await body(request);
      return json(await db.pushEvents(b.source, b.events));
    }
    // Events added on the Kindle (kept here, no computer needed). Body: title, date
    // "YYYY-MM-DD", and either all_day or time "HH:MM" + minutes; local to the desk's TZ.
    if (path === "/api/events" && method === "POST") return json(await db.addEvent(await body(request)));
    if ((m = path.match(/^\/api\/events\/(\d+)$/)) && method === "DELETE") {
      await db.removeEvent(+m[1]);
      return json({ ok: true });
    }
    if (path === "/api/calendar/refresh" && method === "POST") {
      return json({ errors: (await db.refreshCalendars()).errors });
    }

    // Kindle app updates: which build is current (the file itself is a static asset).
    if (path === "/api/kindle/release" && method === "GET") return json(release);
    // Problems the Kindle reports (e.g. a screen that crashed), newest first, last 20.
    if (path === "/api/kindle/log" && method === "POST") {
      const text = String((await body(request)).text || "").slice(0, 4000);
      const log = ((await db.get("kindle_log")) || []).slice(0, 19);
      log.unshift({ at: new Date().toISOString(), text });
      await db.set("kindle_log", log);
      return json({ ok: true });
    }
    if (path === "/api/kindle/log" && method === "GET") return json((await db.get("kindle_log")) || []);

    // Weather
    if (path === "/api/weather" && method === "GET") return json(await weather(db, request));
    // Places matching a name, to pick one on the Kindle: [{lat, lon, name, area}].
    if (path === "/api/weather/places" && method === "GET") return json(await findPlaces(url.searchParams.get("q") || ""));
    if (path === "/api/weather/location" && method === "PUT") {
      const b = await body(request);
      const loc = b.lat != null ? { lat: +b.lat, lon: +b.lon, name: String(b.name || "Here").slice(0, 40) } : await findPlace(String(b.place || ""));
      if (!loc || !Number.isFinite(loc.lat)) throw new HttpError(400, "Couldn't find that place.");
      await db.set("weather_loc", loc);
      return json(loc);
    }

    // Deck: the Kindle queues a tile; the helper on that computer picks it up and opens the app.
    if (path === "/api/deck" && method === "GET") return json(await db.deckConfig());
    // App icons for the tiles: the computer's helper uploads them (small greyscale PNGs,
    // base64, keyed by tile label); the Kindle fetches them when icons_version changes.
    if (path === "/api/deck/icons" && method === "POST") {
      const b = await body(request);
      if (!(b.machine in MACHINES) || typeof b.icons !== "object") throw new HttpError(400, "Send {machine, icons: {label: base64 png}, apps: [names]}.");
      if (b.machine === "pc" && (b.os === "linux" || b.os === "windows")) {
        await db.set("pc_os", b.os);
      }
      let seeded = false;
      // Every app on that computer (the Kindle's "All apps" view); names only, sorted.
      if (Array.isArray(b.apps)) {
        const apps = [...new Set(b.apps.map((a) => String(a).slice(0, 60)).filter((a) => /^[\w .&'+()-]{1,60}$/.test(a)))]
          .sort((x, y) => x.localeCompare(y)).slice(0, 300);
        await db.set(`deck_apps_${b.machine}`, apps);
        // The PC's favourites start as a guess; once its apps are known, the common ones it
        // really has replace them. Only while untouched, never the Mac's, and only from a
        // helper that sends Windows' whole "All apps" (all_apps; 0.7 sent Start menu shortcuts).
        if (b.machine === "pc" && b.all_apps === true) {
          const deck = await db.get("deck");
          const tiles = starterTiles(apps);
          if ((!deck || JSON.stringify(deck.home) === JSON.stringify(DEFAULT_DECK.home)) && tiles.length) {
            await db.set("deck", { ...(deck || DEFAULT_DECK), home: tiles });
            seeded = true;
          }
        }
      }
      const icons = Object.fromEntries(Object.entries(b.icons).slice(0, 320)
        .filter(([, v]) => typeof v === "string" && v.length < 40000).map(([k, v]) => [String(k).slice(0, 40), v]));
      const digest = await crypto.subtle.digest("SHA-1", new TextEncoder().encode(JSON.stringify(icons)));
      const version = [...new Uint8Array(digest)].slice(0, 8).map((x) => x.toString(16).padStart(2, "0")).join("");
      if ((await db.get(`deck_icons_ver_${b.machine}`)) !== version) {
        await db.set(`deck_icons_${b.machine}`, icons);
        await db.set(`deck_icons_ver_${b.machine}`, version);
      }
      return json({ ok: true, version, icons: Object.keys(icons).length, seeded });
    }
    if (path === "/api/deck/icons" && method === "GET") {
      const machine = url.searchParams.get("machine");
      return json({ version: (await db.get(`deck_icons_ver_${machine}`)) || "", icons: (await db.get(`deck_icons_${machine}`)) || {} });
    }
    if (path === "/api/deck" && method === "PUT") {
      const b = await body(request);
      for (const p of PROFILES) if (!Array.isArray(b[p])) throw new HttpError(400, "Send {work: [...tiles], home: [...tiles]}.");
      await db.set("deck", b);
      return json({ ok: true });
    }
    // Join a meeting from the Kindle's heads-up: the computer in use opens the call's link.
    if (path === "/api/join" && method === "POST") {
      const link = String((await body(request)).url || "");
      if (!MEETING.test(link) || (link.match(MEETING) || [""])[0] !== link) throw new HttpError(400, "Not a meeting link.");
      const kv = await db.all();
      const [p] = await db.resolve(now(), kv);
      const machine = MACHINE_FOR[p] || "mac";
      const item = { id: crypto.randomUUID(), machine, label: "Meeting", url: link, at: now() };
      const q = (kv.deck_queue || []).filter((x) => now() - x.at < 30);
      q.push(item);
      await db.set("deck_queue", q);
      await db.set("deck_last", { id: item.id, label: "Meeting", machine, state: "sent", at: now() });
      return json({ ok: true, machine });
    }
    // The Kindle asks the Mac to move Desk Buddy: {to: "cloud" | "local"}. Rides the Deck queue
    // (helper.py's long-poll), so the Mac hears it at once; it switches, copies and leaves a note.
    if (path === "/api/kindle/switch" && method === "POST") {
      const to = (await body(request)).to;
      if (!["cloud", "local"].includes(to)) throw new HttpError(400, "to must be cloud or local");
      const q = ((await db.get("deck_queue")) || []).filter((x) => now() - x.at < 30 && !x.switch); // a newer ask replaces an older one
      q.push({ id: crypto.randomUUID(), machine: "mac", label: "Switch", switch: to, at: now() });
      await db.set("deck_queue", q);
      return json({ ok: true });
    }
    if (path === "/api/deck/run" && method === "POST") {
      const b = await body(request);
      return json(await db.deckRun(b.profile, Number(b.index), b.app ? String(b.app) : null));
    }
    // Demo mode for photos: placeholder tasks, calendar and Claude status.
    if (path === "/api/demo" && method === "POST") {
      const on = !!(await body(request)).on;
      if (on) await db.seedDemo();
      await db.set("demo", on);
      return json({ demo: on });
    }

    // Settings: {key, value}
    if (path === "/api/settings" && method === "GET") return json(await db.settings());
    // The Kindle's own settings: {values: {theme: "dark", …}} merges; the newest change wins
    if (path === "/api/kindle/settings" && method === "GET") return json((await db.get("kindle_settings")) || { values: {}, at: 0 });
    if (path === "/api/kindle/settings" && method === "POST") {
      const b = await body(request);
      const vals = Object.fromEntries(Object.entries(b.values || {}).filter(([k, v]) => KINDLE_SETTINGS[k]?.(v)));
      const cur = (await db.get("kindle_settings")) || { values: {}, at: 0 };
      const next = { values: { ...cur.values, ...vals }, at: Date.now() / 1000 };
      await db.set("kindle_settings", next);
      return json(next);
    }
    // How the Kindle is (battery, version, last seen) and whether it has the latest build
    if (path === "/api/kindle/status" && method === "GET") {
      const st = (await db.get("kindle_status")) || null;
      return json({ ...(st || {}), seen: !!st, latest: release.sha256 || "", up_to_date: !!st && !!st.sha && st.sha === release.sha256 });
    }
    if (path === "/api/settings" && method === "POST") {
      const b = await body(request);
      const def = SETTINGS[b.key];
      if (!def || !def.ok(b.value)) throw new HttpError(400, "Unknown setting or value.");
      const cur = (await db.get("settings")) || {};
      await db.set("settings", { ...cur, [b.key]: b.value });
      if (b.key === "album_covers" && !b.value) await db.clearArt();
      if (b.key === "task_lists" && b.value === "one") await db.mergeLists();
      return json(await db.settings());
    }

    // Music: the helper reports what's playing; the Kindle sends play/pause/next/previous.
    if (path === "/api/music/state" && method === "POST") {
      const b = await body(request);
      if (!(b.machine in MACHINES)) throw new HttpError(400, "machine must be mac or pc");
      const str = (v, n) => String(v ?? "").slice(0, n);
      await db.set(`music_${b.machine}`, { app: str(b.app, 30), title: str(b.title, 120), artist: str(b.artist, 120),
        album: str(b.album, 120), playing: !!b.playing, running: b.running !== false, at: now(),
        volume: Number.isInteger(b.volume) ? Math.max(0, Math.min(100, b.volume)) : null, // the computer's output volume
        // where the computer's sound can go (only sent when there's a choice)
        outputs: (Array.isArray(b.outputs) ? b.outputs : []).slice(0, 10).map((o) => ({
          id: str(o.id, 120), name: str(o.name, 60), kind: str(o.kind, 12), current: !!o.current, connected: o.connected !== false })) });
      if (b.running === false || !b.title) await db.clearArt(); // closed, or nothing loaded: the cover goes too
      return json({ ok: true });
    }
    // Album cover: only the current song's, one small image, overwritten each time.
    if (path === "/api/music/art" && method === "POST") {
      const b = await body(request);
      const image = String(b.image || "");
      if (!image || image.length > 60000) throw new HttpError(400, "Send {key, image: base64} (small).");
      await db.set("music_art", { key: String(b.key || "").slice(0, 200), image });
      await db.set("music_art_key", String(b.key || "").slice(0, 200));
      return json({ ok: true });
    }
    if (path === "/api/music/art" && method === "GET") return json((await db.get("music_art")) || {});
    if (path === "/api/music" && method === "POST") {
      const b = await body(request);
      const pick = /^(output|connect):[^\s]{1,120}$/.test(String(b.action || "")) // switch the computer's sound output
        || /^volume:(up|down)$/.test(String(b.action || "")); // or turn its volume up / down
      if (!pick && !["playpause", "next", "previous"].includes(b.action)) throw new HttpError(400, "action must be playpause, next, previous or output:<id>");
      const kv = await db.all();
      const m = musicNow(kv);
      const machine = m?.machine || "mac";
      const q = ((kv.deck_queue) || []).filter((x) => now() - x.at < 30);
      q.push({ id: crypto.randomUUID(), machine, label: "Music", media: b.action, at: now() });
      await db.set("deck_queue", q);
      return json({ ok: true, machine });
    }

    // Pomodoro: {action: start|switch|break|pause|resume|stop|sound, task?, minutes?, on?}
    if (path === "/api/pomodoro" && method === "POST") return json(await db.pomodoro(await body(request)));
    if (path === "/api/pomodoro" && method === "GET") return json(await db.pomoInfo());

    // Favourites: {profile, app} adds an app from that computer's list; {profile, remove: index} removes a tile.
    if (path === "/api/deck/favourite" && method === "POST") {
      return json(await db.deckFavourite(await body(request)));
    }
    if (path === "/api/deck/next" && method === "GET") {
      // ?wait=20 holds the request until a tap arrives (up to 20 s), so apps open
      // within a fraction of a second instead of at the next check.
      const machine = url.searchParams.get("machine");
      const until = Date.now() + Math.min(25, Number(url.searchParams.get("wait")) || 0) * 1000;
      for (;;) {
        const item = await db.deckNext(machine);
        // The timer rides along so the computer can chime when a focus or break ends.
        if (item || Date.now() >= until || request.signal?.aborted) {
          const set = await db.settings();
          const wanted = set.music_sync && ((await db.get("music_wanted")) || 0) > now();
          return json({ ...(item || {}), pomodoro: await db.pomoInfo(), music_wanted: wanted, album_covers: set.album_covers, music_app: set.music_app });
        }
        await new Promise((r) => setTimeout(r, 300));
      }
    }
    if (path === "/api/deck/done" && method === "POST") {
      await db.deckDone(await body(request));
      return json({ ok: true });
    }

    if (path === "/api/message" && method === "POST") {
      const t = String((await body(request)).text ?? "").trim().slice(0, 40);
      await db.set("message", t ? { text: t } : null);
      return json({ message: t || null });
    }

    // Form actions from the phone page
    if (method === "POST" && path.startsWith("/do/")) {
      const form = await request.formData().catch(() => new FormData());
      const r = String(form.get("ret") || "");
      const ret = r === "/kindle" ? "/kindle" : /^\/app\?list=(work|home)$/.test(r) ? r : "/app";
      if ((m = path.match(/^\/do\/(toggle|pin|delete)\/(\d+)$/))) {
        await { toggle: () => db.toggle(+m[2]), pin: () => db.pin(+m[2]), delete: () => db.del(+m[2]) }[m[1]]();
      } else if ((m = path.match(/^\/do\/move\/(\d+)\/(-?\d+)$/))) {
        await db.move(+m[1], +m[2] > 0 ? 1 : -1);
      } else if (path === "/do/add") {
        await db.addTask(form.get("profile") || "work", form.get("text") || "", null, +form.get("parent_id") || null);
      } else if ((m = path.match(/^\/do\/notes\/(\d+)$/))) {
        await db.update(+m[1], { notes: form.get("notes") || "" });
      } else if (path === "/do/cal-add") {
        await db.addCalendar({ name: form.get("name"), url: form.get("url"), profile: form.get("profile") });
      } else if ((m = path.match(/^\/do\/cal-del\/(\d+)$/))) {
        await db.removeCalendar(+m[1]);
      } else if ((m = path.match(/^\/do\/mode\/(\w+)$/))) {
        await db.setMode(m[1]);
      } else if (path === "/do/clear-message") {
        await db.set("message", null);
      } else if (path === "/do/wake") {
        await db.set("wake", now() + 600);
      } else {
        throw new HttpError(404, "Not found");
      }
      if (request.headers.get("x-requested-with") === "xhr") return new Response(null, { status: 204 });
      return redirect(ret);
    }
    throw new HttpError(404, "Not found");
  })();

  // Anything that changes data tells a waiting Kindle (GET /api/desk?wait). Heartbeats only when
  // the list or light / dark changed (above); logs and the computers' own reports don't count.
  if (method !== "GET" && res.status < 400 && (path.startsWith("/api/") || path.startsWith("/do/"))
      && !/^\/api\/(heartbeat|kindle\/log|pair|deck\/icons|helper\/log|connect)/.test(path)) {
    await db.changed();
  }

  if (pair) {
    res.headers.append("Set-Cookie", `${COOKIE}=${env.KEY}; Max-Age=31536000; Path=/; HttpOnly;${url.protocol === "https:" ? " Secure;" : ""} SameSite=Lax`); // local mode is plain http
  }
  return res;
}

// A "moved to" note stays until the computer moves back here (it clears it): a server with a
// note is retired, so a Kindle on Auto follows it and never returns to an old copy.
const moveNote = (m) => (m && m.server ? { server: m.server, name: m.name || "" } : null);

// ---------- storage and rules ----------

const now = () => Date.now() / 1000;

class Store {
  constructor(db, tz, ctx) {
    this.db = db;
    this.tz = tz;
    this.ctx = ctx;
  }

  async get(k) {
    const r = await this.db.prepare("select v from kv where k=?").bind(k).first();
    return r ? JSON.parse(r.v) : null;
  }

  // "Something changed" stamp for waiting Kindles.
  async changed() {
    await this.set("rev", Date.now());
  }

  async set(k, v) {
    await this.db.prepare("insert into kv(k,v) values(?,?) on conflict(k) do update set v=excluded.v").bind(k, JSON.stringify(v)).run();
  }

  async all() {
    // Everything except the icon images, which only GET /api/deck/icons needs.
    const { results } = await this.db.prepare("select k, v from kv where k not in ('deck_icons_mac', 'deck_icons_pc', 'weather_cache', 'kindle_log', 'music_art')").all();
    return Object.fromEntries(results.map((r) => [r.k, JSON.parse(r.v)]));
  }

  async tasks(profile) {
    const q = profile
      ? this.db.prepare("select * from tasks where profile=? order by pos, id").bind(profile)
      : this.db.prepare("select * from tasks order by pos, id");
    return (await q.all()).results;
  }

  // parentId makes it a subtask (one level deep; it joins its parent's profile).
  async addTask(profile, text, tag = null, parentId = null, notes = null) {
    text = String(text).trim().slice(0, 120);
    if (parentId) {
      const p = await this.db.prepare("select profile, parent_id from tasks where id=?").bind(parentId).first();
      if (!p || p.parent_id) throw new HttpError(400, "Subtasks go under a top-level task.");
      profile = p.profile;
    }
    if (!PROFILES.includes(profile) || !text) throw new HttpError(400, "Need a task name and a profile (work or home).");
    const r = await this.db
      .prepare("select coalesce(max(pos), 0) + 1 as p from tasks where profile=? and parent_id is ?")
      .bind(profile, parentId)
      .first();
    const res = await this.db
      .prepare("insert into tasks(profile,text,tag,pos,created,parent_id,notes) values(?,?,?,?,?,?,?)")
      .bind(profile, text, tag, r.p, now(), parentId, notes ? String(notes).slice(0, 1000) : null)
      .run();
    return res.meta.last_row_id;
  }

  async update(id, b) {
    const sets = [], vals = [];
    if (typeof b.text === "string" && b.text.trim()) sets.push("text=?"), vals.push(b.text.trim().slice(0, 120));
    if (typeof b.notes === "string") sets.push("notes=?"), vals.push(b.notes.trim().slice(0, 1000) || null);
    if (b.tag !== undefined) sets.push("tag=?"), vals.push(b.tag ? String(b.tag).slice(0, 20) : null);
    if (!sets.length) throw new HttpError(400, "Nothing to change: send text, notes or tag.");
    await this.db.prepare(`update tasks set ${sets.join(",")} where id=?`).bind(...vals, id).run();
  }

  async toggle(id) {
    await this.db
      .prepare("update tasks set done = 1 - done, pinned = case when done = 0 then 0 else pinned end, done_at = case when done = 0 then ? else null end where id=?")
      .bind(now(), id)
      .run();
    // Its subtasks follow: ticking a task ticks them all, unticking it unticks them.
    await this.db
      .prepare("update tasks set done = (select done from tasks where id=?1), pinned = 0, done_at = (select done_at from tasks where id=?1) where parent_id=?1")
      .bind(id)
      .run();
    // Finishing the task you're focusing on: the next one moves in and the timer carries on.
    const p = await this.get("pomodoro");
    if (p && p.task === id && p.state !== "idle") {
      const t = await this.db.prepare("select profile, done from tasks where id=?").bind(id).first();
      if (t && t.done) {
        const next = (await this.tasks(t.profile)).find((x) => !x.done && !x.parent_id && x.id !== id);
        if (next) await this.pin(next.id);
        await this.set("pomodoro", { ...p, task: next ? next.id : null });
      }
    }
  }

  // ----- pomodoro -----

  async seedDemo() {
    const r = await this.db.prepare("select count(*) as n from tasks where profile like '%_demo'").first();
    if (r.n > 0) return;
    for (const [p, list] of Object.entries(DEMO_TASKS)) {
      let pos = 1;
      for (const t of list) {
        const res = await this.db.prepare("insert into tasks(profile,text,done,pinned,tag,pos,created,notes) values(?,?,?,?,?,?,?,?)")
          .bind(p + "_demo", t.text, t.done || 0, t.pinned || 0, t.tag || null, pos++, now(), t.notes || null).run();
        let k = 1;
        for (const [text, done] of t.kids || []) {
          await this.db.prepare("insert into tasks(profile,text,done,pos,created,parent_id) values(?,?,?,?,?,?)")
            .bind(p + "_demo", text, done, k++, now(), res.meta.last_row_id).run();
        }
      }
    }
  }

  // The Music widget is on screen: keep "music wanted" alive for a minute. Writes at most
  // every 30 s; when it switches on, a wake-up item makes the computers notice at once.
  async wantMusic() {
    const until = (await this.get("music_wanted")) || 0;
    if (until - now() > 30) return;
    await this.set("music_wanted", now() + 60);
    if (until < now()) {
      const q = ((await this.get("deck_queue")) || []).filter((x) => now() - x.at < 30);
      for (const machine of Object.keys(MACHINES)) q.push({ id: crypto.randomUUID(), machine, wake: true, at: now() });
      await this.set("deck_queue", q);
    }
  }

  // One list for everything: the Windows list joins the Mac's (after it), and any
  // manual choice of list ends.
  async mergeLists() {
    const top = await this.db.prepare("select coalesce(max(pos), 0) as m from tasks where profile='work'").first("m");
    await this.db.batch([
      this.db.prepare("update tasks set profile='work', pos=pos+? where profile='home'").bind(top + 1),
      this.db.prepare("delete from kv where k='override'"),
    ]);
  }

  async settings() {
    const cur = (await this.get("settings")) || {};
    return Object.fromEntries(Object.entries(SETTINGS).map(([k, d]) => [k, cur[k] ?? d.def]));
  }

  // From the Kindle's desk requests (?kb=battery&kc=charging&kv=version&ks=build sha): saved
  // when something changed, else at most every 2 minutes (for "seen just now").
  async kindleSeen(q) {
    const st = { battery: +q.get("kb"), charging: q.get("kc") === "1", version: String(q.get("kv") || "").slice(0, 40), sha: String(q.get("ks") || "").slice(0, 64) };
    const cur = (await this.get("kindle_status")) || {};
    const same = cur.battery === st.battery && cur.charging === st.charging && cur.version === st.version && cur.sha === st.sha;
    if (!same || now() - (cur.at || 0) > 120) await this.set("kindle_status", { ...st, at: now() });
  }

  async clearArt() {
    await this.db.prepare("delete from kv where k in ('music_art', 'music_art_key')").run();
  }

  async pomoInfo() {
    const p = (await this.get("pomodoro")) || { state: "idle" };
    const sound = (await this.get("pomo_sound")) ?? true;
    let title = null;
    if (p.task) title = (await this.db.prepare("select text from tasks where id=?").bind(p.task).first())?.text ?? null;
    return { ...p, title, sound, now: Math.floor(now()) };
  }

  async pomodoro(b) {
    const p = (await this.get("pomodoro")) || { state: "idle" };
    const t = Math.floor(now());
    const mins = (m, d) => Math.max(1, Math.min(120, Number(m) || d));
    switch (b.action) {
      case "start": { // a focus session, on a task (pinned) or the pinned one
        let task = b.task ? Number(b.task) : p.task || null;
        if (task) await this.pin(task);
        const length = mins(b.minutes, (await this.settings()).focus_min) * 60;
        await this.set("pomodoro", { state: "focus", task, length, ends: t + length });
        break;
      }
      case "switch": { // work on another task; the timer carries on
        const task = Number(b.task);
        if (!task) throw new HttpError(400, "Which task?");
        await this.pin(task);
        await this.set("pomodoro", { ...p, task });
        break;
      }
      case "break": {
        const length = mins(b.minutes, (await this.settings()).break_min) * 60;
        await this.set("pomodoro", { state: "break", task: p.task || null, length, ends: t + length });
        break;
      }
      case "pause":
        if (p.state === "focus" || p.state === "break") {
          await this.set("pomodoro", { ...p, state: "paused", was: p.state, left: Math.max(0, p.ends - t) });
        }
        break;
      case "resume":
        if (p.state === "paused") {
          await this.set("pomodoro", { ...p, state: p.was || "focus", ends: t + (p.left || 0), left: undefined, was: undefined });
        }
        break;
      case "stop":
        await this.set("pomodoro", { state: "idle", task: p.task || null });
        break;
      case "sound":
        await this.set("pomo_sound", !!b.on);
        break;
      default:
        throw new HttpError(400, "action must be start, switch, break, pause, resume, stop or sound");
    }
    return this.pomoInfo();
  }

  async pin(id) {
    const r = await this.db.prepare("select profile from tasks where id=?").bind(id).first();
    if (!r) return;
    await this.db.batch([
      this.db.prepare("update tasks set pinned=0 where profile=?").bind(r.profile),
      this.db.prepare("update tasks set pinned=1 where id=?").bind(id),
    ]);
  }

  async unpin(profile) {
    await this.db.prepare("update tasks set pinned=0 where profile=?").bind(profile).run();
  }

  // Moves a task one place up or down among its siblings.
  // Drag and drop (the Mac helper): put a task before another (or at the end), at the top
  // level (parent null) or inside a group (as a subtask of a top-level task). A task with
  // subtasks of its own stays at the top level. Positions are renumbered in order.
  async place(id, parent, before) {
    const me = await this.db.prepare("select id, profile, parent_id from tasks where id=?").bind(id).first();
    if (!me) throw new HttpError(404, "No such task.");
    parent = parent ? +parent : null;
    if (parent) {
      const p = await this.db.prepare("select profile, parent_id from tasks where id=?").bind(parent).first();
      if (!p || p.parent_id || p.profile !== me.profile || parent === id) throw new HttpError(400, "Subtasks go under a top-level task on the same list.");
      const kids = await this.db.prepare("select count(*) as n from tasks where parent_id=?").bind(id).first();
      if (kids.n > 0) throw new HttpError(400, "A task with subtasks can't become a subtask.");
    }
    const siblings = (await this.tasks(me.profile)).filter((t) => (t.parent_id ?? null) === parent && t.id !== id).map((t) => t.id);
    const at = before ? siblings.indexOf(+before) : -1;
    siblings.splice(at < 0 ? siblings.length : at, 0, id);
    await this.db.batch([
      this.db.prepare("update tasks set parent_id=? where id=?").bind(parent, id),
      ...siblings.map((x, n) => this.db.prepare("update tasks set pos=? where id=?").bind(n, x)),
    ]);
  }

  async move(id, dir) {
    const r = await this.db.prepare("select profile, parent_id from tasks where id=?").bind(id).first();
    if (!r) return;
    const ids = (await this.tasks(r.profile)).filter((t) => t.parent_id === r.parent_id).map((t) => t.id);
    const i = ids.indexOf(id), j = i + dir;
    if (j < 0 || j >= ids.length) return;
    [ids[i], ids[j]] = [ids[j], ids[i]];
    await this.db.batch(ids.map((x, n) => this.db.prepare("update tasks set pos=? where id=?").bind(n, x)));
  }

  async exportAll() {
    const tasks = (await this.db.prepare("select * from tasks order by id").all()).results;
    const kv = (await this.db.prepare("select k, v from kv").all()).results;
    return { version: 1, at: now(), tasks, kv };
  }

  // Replaces everything with an export. Big values (icons) come too, so the deck looks the same.
  async importAll(b) {
    if (b?.version !== 1 || !Array.isArray(b.tasks) || !Array.isArray(b.kv)) throw new HttpError(400, "Not a Desk Buddy export.");
    const cols = ["id", "profile", "text", "done", "pinned", "tag", "pos", "created", "parent_id", "notes", "done_at"];
    const stmts = [this.db.prepare("delete from tasks"), this.db.prepare("delete from kv")];
    for (const t of b.tasks) stmts.push(this.db.prepare(`insert into tasks(${cols}) values(${cols.map(() => "?")})`).bind(...cols.map((c) => t[c] ?? null)));
    for (const r of b.kv) if (typeof r.k === "string" && typeof r.v === "string") stmts.push(this.db.prepare("insert into kv(k, v) values(?, ?)").bind(r.k, r.v));
    await this.db.batch(stmts);
    return { tasks: b.tasks.length, kv: b.kv.length };
  }

  async clear(profile, doneOnly) {
    if (!PROFILES.includes(profile)) throw new HttpError(400, "profile must be work or home");
    const r = doneOnly
      ? await this.db.prepare("delete from tasks where profile=? and (done=1 or parent_id in (select id from tasks where profile=? and done=1))").bind(profile, profile).run()
      : await this.db.prepare("delete from tasks where profile=?").bind(profile).run();
    return r.meta?.changes ?? 0;
  }

  async del(id) {
    await this.db.prepare("delete from tasks where id=? or parent_id=?").bind(id, id).run();
  }

  // ----- calendars -----

  async addCalendar(c) {
    const url = String(c.url || "").trim();
    if (!/^(https?|webcal):\/\/\S+$/i.test(url)) throw new HttpError(400, "Paste the calendar's private ICS link (starts with https:// or webcal://).");
    const profile = ["work", "home", "both"].includes(c.profile) ? c.profile : "both";
    const list = (await this.get("calendars")) || [];
    list.push({ name: String(c.name || "Calendar").trim().slice(0, 30) || "Calendar", url, profile });
    await this.set("calendars", list);
    await this.refreshCalendars();
  }

  async removeCalendar(i) {
    const list = (await this.get("calendars")) || [];
    list.splice(i, 1);
    await this.set("calendars", list);
    await this.refreshCalendars();
  }

  async refreshCalendars() {
    const c = { at: now(), ...(await loadCalendars((await this.get("calendars")) || [], now())) };
    await this.set("cal_cache", c);
    return c;
  }

  // Events sent by a computer's calendar app, replacing that computer's previous set.
  async pushEvents(source, events) {
    if (source !== "mac" && source !== "pc") throw new HttpError(400, "source must be mac or pc");
    if (!Array.isArray(events)) throw new HttpError(400, "events must be a list");
    const str = (v, n) => String(v ?? "").slice(0, n);
    const clean = events.slice(0, 3000).filter((e) => Number.isFinite(+e.start) && Number.isFinite(+e.end)).map((e) => ({
      title: str(e.title, 120) || "(no title)", start: Math.floor(+e.start), end: Math.floor(+e.end), all_day: !!e.all_day,
      location: str(e.location, 120), description: str(e.description, 600), cal: str(e.cal, 40), profile: "both",
      url: str(e.url, 300),
    }));
    await this.set(`cal_push_${source}`, { at: now(), events: clean });
    return { ok: true, events: clean.length };
  }

  // An event made on the Kindle. Times are the desk's local time (DST-safe here,
  // rather than on the Kindle). Ended ones older than 60 days are dropped.
  async addEvent(b) {
    const title = String(b.title || "").trim().slice(0, 120);
    const dm = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(b.date || ""));
    if (!title || !dm) throw new HttpError(400, "title and date (YYYY-MM-DD) needed");
    const [y, mo, d] = [+dm[1], +dm[2], +dm[3]];
    let start, end;
    if (b.all_day) {
      start = Date.UTC(y, mo - 1, d) / 1000; // all-day events are UTC midnights, end exclusive
      end = start + 86400;
    } else {
      const tm = /^(\d{1,2}):(\d{2})$/.exec(String(b.time || ""));
      if (!tm) throw new HttpError(400, "time (HH:MM) needed");
      const wall = Date.UTC(y, mo - 1, d, +tm[1], +tm[2]) / 1000;
      start = wall - local(wall, this.tz).offset;
      start = wall - local(start, this.tz).offset; // again, in case the guess crossed a clock change
      end = start + Math.max(5, Math.min(24 * 60, +b.minutes || 60)) * 60;
    }
    const list = ((await this.get("desk_events")) || []).filter((e) => e.end > now() - 60 * 86400);
    const id = Math.max(0, ...list.map((e) => e.id)) + 1;
    const e = { id, title, start, end, all_day: !!b.all_day, location: "", description: "", cal: "Added on the Kindle", profile: "both" };
    list.push(e);
    await this.set("desk_events", list);
    return e;
  }

  async removeEvent(id) {
    await this.set("desk_events", ((await this.get("desk_events")) || []).filter((e) => e.id !== id));
  }

  // Cached events (calendar links refreshed in the background every 10 minutes),
  // plus whatever the computers' calendar apps have sent, plus events added on the Kindle.
  async calendarCache(kv = null) {
    const merged = (c) => {
      const pushed = [...["mac", "pc"].flatMap((m) => kv?.[`cal_push_${m}`]?.events || []), ...(kv?.desk_events || [])];
      // The same event often comes from several accounts (holidays, shared invites):
      // same title, start and end counts as one.
      const seen = new Set();
      // Settings › Calendar on the Kindle: each calendar on both lists (default), one, or off
      const prefs = kv?.cal_prefs || {};
      const events = [...c.events, ...pushed].sort((a, b) => a.start - b.start).filter((e) => {
        const k = `${e.title.trim().toLowerCase()}|${e.start}|${e.end}|${e.all_day}`;
        return seen.has(k) ? false : seen.add(k);
      }).filter((e) => prefs[e.cal] !== "off")
        .map((e) => (prefs[e.cal] === "work" || prefs[e.cal] === "home" ? { ...e, profile: prefs[e.cal] } : e))
        // one computer so far: one list, so every calendar shows on it
        .map((e) => (kv && this.onlyList(kv) && e.profile !== "both" ? { ...e, profile: "both" } : e));
      return { events, errors: c.errors || [] };
    };
    if (!kv) kv = await this.all();
    if (kv.demo) return { events: demoEvents(now(), local(now(), this.tz).offset), errors: [] };
    const c = kv.cal_cache;
    const busy = kv.cal_busy;
    if (stale(c, now()) && (!busy || now() - busy > 60)) {
      await this.set("cal_busy", now());
      const job = this.refreshCalendars();
      if (this.ctx && c) this.ctx.waitUntil(job);
      else return merged(await job);
    }
    return merged(c || { events: [], errors: [] });
  }

  // ----- deck -----

  async deckConfig() {
    return (await this.get("deck")) || DEFAULT_DECK;
  }

  async deckFavourite(b) {
    const profile = b.profile;
    if (!PROFILES.includes(profile)) throw new HttpError(400, "profile must be work or home");
    const machine = MACHINE_FOR[profile];
    const deck = structuredClone(await this.deckConfig());
    const tiles = deck[profile] || (deck[profile] = []);
    if (b.move != null) { // {move: from, to}
      const i = Number(b.move), j = Number(b.to);
      if (!(i >= 0 && i < tiles.length && j >= 0 && j < tiles.length)) throw new HttpError(404, "No such tile.");
      tiles.splice(j, 0, tiles.splice(i, 1)[0]);
    } else if (b.remove != null) {
      const i = Number(b.remove);
      if (!(i >= 0 && i < tiles.length)) throw new HttpError(404, "No such tile.");
      tiles.splice(i, 1);
    } else {
      const app = String(b.app || "");
      if (!((await this.get(`deck_apps_${machine}`)) || []).includes(app)) throw new HttpError(404, "That app isn't on the list.");
      if (!tiles.some((t) => t.label === app || t.app === app || t.win === app)) {
        tiles.push(machine === "mac" ? { label: app, app } : { label: app, win: app });
      }
    }
    await this.set("deck", deck);
    return { ok: true, tiles: tiles.length };
  }

  // Runs a favourite tile (index) or, from "All apps", any app on that computer's list (app).
  async deckRun(profile, index, appName) {
    if (!PROFILES.includes(profile)) throw new HttpError(400, "profile must be work or home");
    const machine = MACHINE_FOR[profile];
    let tile;
    if (appName) {
      if (!((await this.get(`deck_apps_${machine}`)) || []).includes(appName)) throw new HttpError(404, "That app isn't on the list.");
      tile = machine === "mac" ? { label: appName, app: appName } : { label: appName, win: appName };
    } else {
      tile = (await this.deckConfig())[profile]?.[index];
    }
    if (!tile) throw new HttpError(404, "No such tile.");
    const item = { id: crypto.randomUUID(), machine, label: tile.label, app: tile.app || null, win: tile.win || null, at: now() };
    const q = ((await this.get("deck_queue")) || []).filter((x) => now() - x.at < 30);
    q.push(item);
    await this.set("deck_queue", q);
    await this.set("deck_last", { id: item.id, label: tile.label, machine, state: "sent", at: now() });
    return { ok: true };
  }

  async deckNext(machine) {
    if (!(machine in MACHINES)) throw new HttpError(400, "machine must be mac or pc");
    const q = ((await this.get("deck_queue")) || []).filter((x) => now() - x.at < 30);
    const i = q.findIndex((x) => x.machine === machine);
    if (i < 0) return null;
    const [item] = q.splice(i, 1);
    await this.set("deck_queue", q);
    const last = await this.get("deck_last");
    if (last && last.id === item.id) await this.set("deck_last", { ...last, state: "opening", at: now() });
    return item;
  }

  async deckDone(b) {
    const last = await this.get("deck_last");
    if (last && last.id === b.id) {
      await this.set("deck_last", { ...last, state: b.ok ? "done" : "failed", error: b.ok ? null : String(b.error || "").slice(0, 80), at: now() });
    }
  }

  async setMode(mode) {
    if (mode === "auto") return this.set("override", null);
    if (!PROFILES.includes(mode)) throw new HttpError(400, "Mode must be work, home or auto.");
    await this.set("override", { profile: mode, until: next4am(now(), this.tz) });
  }

  // Returns [profile, reason]; profile is work | home | night. Same rules as server.py.
  // Computers with the helper (they've checked in or sent their app list), e.g. ["mac"].
  connectedMachines(kv) {
    return Object.keys(MACHINES).filter((m) => (kv.heartbeats || {})[m] || kv[`deck_apps_${m}`]);
  }

  pcLabel(kv) {
    const os = (kv.heartbeats || {}).pc?.os || kv.pc_os;
    return os === "linux" ? "Linux" : "Windows";
  }

  machineLabel(machine, kv) {
    return machine === "pc" ? this.pcLabel(kv) : MACHINES[machine][1];
  }

  // With fewer than two computers connected there's nothing to switch between: the one
  // computer's list (the Mac's when none has connected yet). null when both are connected.
  onlyList(kv) {
    const c = this.connectedMachines(kv);
    return c.length >= 2 ? null : MACHINES[c[0] || "mac"][0];
  }

  async resolve(t = now(), kv = null) {
    kv = kv || (await this.all());
    const [p, why] = await this.resolveAny(t, kv);
    const only = this.onlyList(kv);
    return only && PROFILES.includes(p) ? [only, why === "manual" ? "only computer" : why] : [p, why];
  }

  async resolveAny(t, kv) {
    const ov = kv.override;
    if (ov && t < ov.until) return [ov.profile, "manual"];
    let best = null;
    for (const [machine, h] of Object.entries(kv.heartbeats || {})) {
      if (!(machine in MACHINES) || t - h.at > 180) continue; // helper hasn't checked in for 3 minutes
      const lastInput = h.at - h.idle;
      if (t - lastInput > 300) continue; // nobody has touched it for 5 minutes
      if (!best || lastInput > best[1]) best = [machine, lastInput];
    }
    const one = ((kv.settings || {}).task_lists || SETTINGS.task_lists.def) === "one";
    if (best) {
      const [profile] = MACHINES[best[0]];
      const label = this.machineLabel(best[0], kv);
      return [one ? "work" : profile, `${label} in use`];
    }
    const l = local(t, this.tz);
    if (l.hour >= 22 || l.hour < 6) return ["night", "nobody around"];
    return this.idleList(kv);
  }

  // Nobody at a computer: the one list, else the list of the computer used last.
  idleList(kv) {
    if (((kv.settings || {}).task_lists || SETTINGS.task_lists.def) === "one") return ["work", "one list"];
    const lastUsed = Object.entries(kv.heartbeats || {}).filter(([m]) => m in MACHINES).sort((a, b) => b[1].at - a[1].at)[0];
    return lastUsed ? [MACHINES[lastUsed[0]][0], `${this.machineLabel(lastUsed[0], kv)} used last`] : ["work", "default"];
  }

  async state() {
    const t = now();
    const kv = await this.all();
    const [profile, reason] = await this.resolve(t, kv);
    const all = await this.tasks();
    return {
      profile, reason,
      pc_label: this.pcLabel(kv),
      mode: kv.override && t < kv.override.until && !this.onlyList(kv) ? kv.override.profile : "auto",
      computers: this.connectedMachines(kv), // fewer than two: no Mac / Windows choice, "Searching…"
      claude: kv.claude ?? null,
      heartbeats: kv.heartbeats || {},
      tasks: Object.fromEntries(PROFILES.map((p) => [p, all.filter((x) => x.profile === p)])),
    };
  }

  // Everything the Kindle app needs to draw one screen.
  async desk() {
    const t = now();
    const kv = await this.all();
    let [profile, why] = await this.resolve(t, kv);
    const night = profile === "night" && t > (kv.wake || 0);
    if (profile === "night" && !night) [profile, why] = [this.idleList(kv)[0], "woken by tap"];
    const mode = kv.override && t < kv.override.until && !this.onlyList(kv) ? kv.override.profile : "auto";
    const reason = { manual: "manual · until 4am", "woken by tap": "woken by tap" }[why] || "auto · " + why;
    // Settings › Tasks › Auto-delete: top-level tasks ticked off over auto_clear_min minutes
    // ago go, with their subtasks (0: on the next fetch, within ~10 s).
    if ((kv.settings || {}).auto_clear) {
      const old = t - 60 * (kv.settings.auto_clear_min ?? SETTINGS.auto_clear_min.def);
      await this.db.batch([
        this.db.prepare("delete from tasks where parent_id in (select id from tasks where parent_id is null and done=1 and done_at < ?)").bind(old),
        this.db.prepare("delete from tasks where parent_id is null and done=1 and done_at < ?").bind(old),
      ]);
    }
    const all = await this.tasks();
    const msg = kv.message;

    // Calendar: what's on now and next, until the end of tomorrow.
    const cal = await this.calendarCache(kv);
    const l = local(t, this.tz);
    const endTomorrow = Date.UTC(l.y, l.mo - 1, l.d + 2) / 1000 - l.offset;
    // All of today (the Day timeline shows the past hours too) and tomorrow.
    const startToday = Date.UTC(l.y, l.mo - 1, l.d) / 1000 - l.offset;
    // every calendar seen (Mac, PC, links, Kindle-made), for Settings › Calendar
    const prefs = kv.cal_prefs || {};
    const calNames = [...new Set([...(kv.cal_cache?.events || []), ...["mac", "pc"].flatMap((m) => kv[`cal_push_${m}`]?.events || []), ...(kv.desk_events || [])]
      .map((e) => e.cal).filter(Boolean))].sort((a, b) => a.localeCompare(b));
    const calendarList = calNames.map((name) => ({ name, show: prefs[name] || "both" }));
    const agenda = cal.events
      .filter((e) => (e.profile === profile || e.profile === "both") && e.end > startToday && e.start < endTomorrow)
      .slice(0, 30)
      .map((e) => ({ ...e, join: meetingLink(e) }));

    // Deck: both computers' tiles (the Kindle can switch between them), and how the
    // last tap went. `deck` itself is the current profile's, for older Kindle builds.
    let last = kv.deck_last || null;
    if (last && last.state === "sent" && t - last.at > 15) last = { ...last, state: "waiting" };
    const deckFor = (p) => {
      const machine = MACHINE_FOR[p];
      const hb = (kv.heartbeats || {})[machine];
      return {
        profile: p,
        machine,
        machine_label: this.machineLabel(machine, kv),
        online: !!hb && t - hb.at < 180,
        tiles: ((kv.deck || DEFAULT_DECK)[p] || []).map((x) => ({ label: x.label, app: x.app || x.win || "" })),
        icons_version: kv[`deck_icons_ver_${machine}`] || "",
        apps: kv[`deck_apps_${machine}`] || [],
        last: last && last.machine === machine && t - last.at < 600 ? last : null,
      };
    };
    // Only computers that have the helper (they've checked in or sent their app list); the
    // Deck opens on the one used most recently. (Work / Home is only about the task lists.)
    // Both computers, each saying whether it has the helper (it's checked in or sent its app
    // list); the one used most recently first. Unconnected ones show "install the helper".
    const hbs = kv.heartbeats || {};
    const decks = PROFILES.map((p) => ({ ...deckFor(p), connected: !!(hbs[MACHINE_FOR[p]] || kv[`deck_apps_${MACHINE_FOR[p]}`]) }));
    decks.sort((a, b) => (b.connected - a.connected) || (((hbs[b.machine] || {}).at || 0) - ((hbs[a.machine] || {}).at || 0)));
    const deck = { ...decks[0], decks };

    return {
      screen: msg ? "message" : night ? "night" : "tasks",
      message: msg ? msg.text : null,
      profile, reason, mode,
      pc_label: this.pcLabel(kv),
      tasks: all
        .filter((x) => x.profile === (kv.demo ? profile + "_demo" : profile))
        .map((x) => ({ id: x.id, text: x.text, done: !!x.done, pinned: !!x.pinned, tag: x.tag, parent_id: x.parent_id ?? null, notes: x.notes ?? "" })),
      open_total: all.filter((x) => !x.done && !x.parent_id && x.profile.endsWith("_demo") === !!kv.demo).length,
      claude: kv.demo ? { state: "working", text: "desk-buddy", at: now() - 540 } : kv.claude ?? null,
      agenda,
      calendar_list: calendarList,
      kindle_settings: kv.kindle_settings || null, // the Kindle's own settings, if set anywhere (newest wins)
      computers: this.connectedMachines(kv), // fewer than two: no Mac / Windows choice, "Searching…"
      calendars: ((kv.calendars || []).filter((c) => c.profile === profile || c.profile === "both")).length
        + (kv.cal_push_mac ? 1 : 0) + (kv.cal_push_pc ? 1 : 0),
      deck,
      // for the Kindle's Auto theme: the computer in use's light/dark appearance (while it
      // checks in), and today's sunrise / sunset from the weather
      appearance: (() => {
        const fresh = Object.values(kv.heartbeats || {}).filter((h) => t - h.at < 180 && typeof h.dark === "boolean").sort((a, b) => b.at - a.at)[0];
        return fresh ? (fresh.dark ? "dark" : "light") : "";
      })(),
      sun: await (async () => {
        const w = await this.get("weather_cache");
        const d = w?.days?.find((x) => x.sunset > t - 86400) || w?.days?.[0];
        return d ? { rise: d.sunrise, set: d.sunset } : null;
      })(),
      pomodoro: await this.pomoInfo(),
      music: musicNow(kv),
      settings: Object.fromEntries(Object.entries(SETTINGS).map(([k, d]) => [k, (kv.settings || {})[k] ?? d.def])),
      now: Math.floor(t),
      utc_offset: l.offset,
    };
  }
}

// ---------- time (the server's clock is UTC; the rules run on UK time) ----------

function local(t, tz) {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-GB", {
      timeZone: tz, hourCycle: "h23", weekday: "short",
      year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
    }).formatToParts(new Date(t * 1000)).map((p) => [p.type, p.value]),
  );
  const y = +parts.year, mo = +parts.month, d = +parts.day, hour = +parts.hour, minute = +parts.minute;
  const offset = Math.round((Date.UTC(y, mo - 1, d, hour, minute, +parts.second) / 1000 - Math.floor(t)) / 60) * 60;
  const weekday = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].indexOf(parts.weekday);
  return { y, mo, d, hour, minute, weekday, offset };
}

function next4am(t, tz) {
  const l = local(t, tz);
  let at = Date.UTC(l.y, l.mo - 1, l.d, 4) / 1000 - l.offset;
  if (at <= t) at += 86400;
  return at - (local(at, tz).offset - l.offset); // in case the clocks change overnight
}

// ---------- phone page ----------

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const title = (s) => s.charAt(0).toUpperCase() + s.slice(1);

// A video-call link in an event (location, notes or link): only these services, so the
// Mac only ever opens meeting pages from a Kindle tap.
const MEETING = /https:\/\/(?:[a-z0-9-]+\.)*(?:zoom\.us|meet\.google\.com|teams\.microsoft\.com|teams\.live\.com|webex\.com|whereby\.com|meet\.jit\.si|facetime\.apple\.com)(?![a-z0-9.-])(?:\/[^\s"'<>()]*)?/i;
const meetingLink = (e) => (`${e.url || ""} ${e.location || ""} ${e.description || ""}`.match(MEETING) || [""])[0].replace(/[.,;]+$/, "");
// The lists' names on the phone page: one per computer (Mac, Windows), or "Tasks" (one list).
const LIST_NAME = { work: "Mac", home: "Windows" };

const APP_CSS = `
@font-face{font-family:Atkinson;src:url(/fonts/AtkinsonHyperlegible-Regular.ttf);font-weight:400}
@font-face{font-family:Atkinson;src:url(/fonts/AtkinsonHyperlegible-Bold.ttf);font-weight:700}
:root{--ink:#000;--paper:#fff;--grey:#4d4d4d;--soft:#8b8b8b;--line:#c4c4c4}
@media (prefers-color-scheme:dark){:root{--ink:#fff;--paper:#000;--grey:#b3b3b3;--soft:#777;--line:#3a3a3a;color-scheme:dark}}
*{box-sizing:border-box}
body{margin:0;background:var(--paper);color:var(--ink);font:17px/1.35 Atkinson,system-ui,sans-serif;-webkit-text-size-adjust:100%}
.w{max-width:620px;margin:0 auto;padding:0 16px 56px}
button,input,textarea{font:inherit;color:inherit}
form{margin:0}.inline{display:inline}
.top{position:sticky;top:0;z-index:2;background:var(--paper);padding:18px 0 12px;border-bottom:2px solid var(--ink)}
.top .r{display:flex;align-items:center;justify-content:space-between;gap:12px}
h1{font-size:30px;line-height:1.1;margin:0}
.status{font-size:13px;color:var(--grey);margin-top:6px}
.label{font-size:12px;font-weight:700;letter-spacing:2px;text-transform:uppercase;color:var(--grey);margin:26px 0 6px}
.seg{display:inline-flex;border:2px solid var(--ink);border-radius:8px;overflow:hidden;flex-shrink:0}
.seg>*{display:block}
.seg a,.seg button{display:block;padding:8px 14px;font-weight:700;font-size:15px;text-decoration:none;color:var(--ink);background:var(--paper);border:0;border-left:2px solid var(--ink);cursor:pointer;min-height:40px}
.seg>:first-child a,.seg>:first-child button,.seg>a:first-child{border-left:0}
.seg .on{background:var(--ink);color:var(--paper)}
.add{display:flex;gap:10px;align-items:center;padding:14px 0 6px}
.add input{flex:1;min-width:0;border:0;border-bottom:2px solid var(--ink);background:none;padding:8px 2px;font-size:18px;border-radius:0;outline:none}
.add input::placeholder{color:var(--soft)}
.plus{width:44px;height:44px;border-radius:50%;border:0;background:var(--ink);color:var(--paper);display:grid;place-items:center;cursor:pointer;flex-shrink:0}
.task{border-bottom:1px solid var(--line)}
.task>summary{list-style:none;display:flex;align-items:flex-start;gap:12px;padding:12px 0;cursor:pointer}
.task>summary::-webkit-details-marker{display:none}
.tick{width:30px;height:30px;border-radius:50%;border:2px solid var(--ink);background:var(--paper);display:grid;place-items:center;padding:0;cursor:pointer;flex-shrink:0}
.done .tick{background:var(--ink);color:var(--paper)}
.t{flex:1;min-width:0;overflow-wrap:anywhere;font-size:18px;padding-top:3px}
.done .t{color:var(--soft);text-decoration:line-through}
.note{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;overflow:hidden;font-size:14px;color:var(--grey);white-space:pre-line;margin-top:2px}
.task[open]>summary .note{display:block}
.pinned{flex-shrink:0;color:var(--ink);padding-top:5px}
.sub{margin-left:42px}.sub .tick{width:24px;height:24px}.sub .t{font-size:16px;padding-top:2px}
.sub>summary{padding:9px 0}
.more{padding:2px 0 16px 42px;display:grid;gap:12px}
.acts{display:flex;gap:8px;flex-wrap:wrap}
.pill{border:2px solid var(--ink);background:var(--paper);border-radius:7px;padding:7px 14px;font-weight:700;font-size:15px;cursor:pointer;min-height:40px}
.pill.solid{background:var(--ink);color:var(--paper)}
textarea{width:100%;border:2px solid var(--line);border-radius:8px;background:none;padding:8px 10px;font-size:15px;resize:vertical}
textarea:focus{border-color:var(--ink);outline:none}
.more .add{padding:0}.more .add input{font-size:16px}
.empty{color:var(--grey);padding:18px 0}
.progress{display:flex;align-items:center;gap:12px;margin-top:18px;font-weight:700;font-size:15px}
.bar{flex:1;height:10px;border:2px solid var(--ink);border-radius:6px;overflow:hidden}.bar i{display:block;height:100%;background:var(--ink)}
.box{border:2px solid var(--ink);border-radius:12px;padding:14px 16px;margin-top:18px}
.box p{margin:4px 0 12px;color:var(--grey);font-size:15px}
.line{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 0;border-bottom:1px solid var(--line)}
.line .h{font-size:13px;color:var(--grey)}
.kindle{display:grid;gap:12px;padding:6px 0 4px}.kindle .h{font-size:13px;color:var(--grey);margin-top:2px}
.kindle .seg{display:flex}.kindle .seg>*{flex:1}.kindle .seg button{width:100%}
details.cal>summary{list-style:none;cursor:pointer;display:flex;justify-content:space-between;align-items:center;padding:14px 0;border-bottom:1px solid var(--line)}
details.cal>summary::-webkit-details-marker{display:none}
details.cal>summary::after{content:"›";font-size:24px;line-height:1;color:var(--grey);transition:transform .15s}
details.cal[open]>summary::after{transform:rotate(90deg)}
details.cal>summary b{display:block}details.cal>summary span{font-size:13px;color:var(--grey)}
.calform{display:grid;gap:10px;padding:12px 0}
.calform input,.calform select{border:2px solid var(--line);border-radius:8px;background:none;padding:9px 10px;font-size:15px;width:100%}
`;

async function phonePage(db, view = "") {
  const s = await db.state();
  const msg = await db.get("message");
  const t = now();
  // one list (a setting), or only one computer with the helper so far: no Mac / Windows switch
  const single = (s.computers || []).length < 2;
  const one = (await db.settings()).task_lists === "one" || single;
  // which list this page shows (its own Mac | Windows switch); the Kindle's own list is separate
  const kindleList = PROFILES.includes(s.profile) ? s.profile : "work";
  const list = single ? kindleList : one ? "work" : PROFILES.includes(view) ? view : kindleList;
  const ret = `<input type="hidden" name="ret" value="/app?list=${list}">`;
  const post = (path, inner, cls = "pill", label = "") =>
    `<form class="inline" method="post" action="${path}">${ret}<button class="${cls}"${label ? ` aria-label="${label}"` : ""}>${inner}</button></form>`;
  const icon = {
    check: '<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8.5l3.2 3L13 4.5" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    pin: '<svg width="18" height="18" viewBox="0 0 18 18" aria-label="Pinned"><path d="M6 2h6l-1 5 3 3H4l3-3zM9 10v6" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/></svg>',
    plus: '<svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true"><path d="M9 3v12M3 9h12" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"/></svg>',
  };
  const tasks = s.tasks[list];
  const row = (x, sub) => {
    const kids = sub ? [] : tasks.filter((c) => c.parent_id === x.id);
    return `<details class="task${x.done ? " done" : ""}${sub ? " sub" : ""}"><summary>
${post(`/do/toggle/${x.id}`, x.done ? icon.check : "", "tick", x.done ? "Mark not done" : "Mark done")}
<span class="t">${esc(x.text)}${x.notes ? `<span class="note">${esc(x.notes)}</span>` : ""}</span>${x.pinned ? `<span class="pinned">${icon.pin}</span>` : ""}</summary>
<div class="more"><div class="acts">
${sub ? "" : post(`/do/pin/${x.id}`, x.pinned ? "Unpin" : "Pin")}
${post(`/do/move/${x.id}/-1`, "Up", "pill", "Move up")}${post(`/do/move/${x.id}/1`, "Down", "pill", "Move down")}
${post(`/do/delete/${x.id}`, "Delete")}</div>
<form method="post" action="/do/notes/${x.id}">${ret}<textarea name="notes" rows="2" maxlength="1000" placeholder="Add a description" aria-label="Description">${esc(x.notes)}</textarea>
<button class="pill">Save description</button></form>
${sub ? "" : `<form class="add" method="post" action="/do/add">${ret}<input type="hidden" name="parent_id" value="${x.id}">
<input type="text" name="text" placeholder="Add a subtask" required maxlength="120" aria-label="Subtask"><button class="plus" aria-label="Add subtask">${icon.plus}</button></form>`}
</div></details>${kids.map((c) => row(c, true)).join("")}`;
  };
  const top = tasks.filter((x) => !x.parent_id);
  const done = top.filter((x) => x.done).length;
  const pcLabel = s.pc_label || "Windows";
  const listNames = { work: "Mac", home: pcLabel };
  const name = single ? listNames[kindleList] : one ? "Tasks" : listNames[list];
  const seen = Object.entries(s.heartbeats).filter(([m]) => m in MACHINES)
    .map(([m, h]) => `${m === "pc" ? pcLabel : MACHINES[m][1]} seen ${ago(t - h.at)}`).join(" · ");
  const listSwitch = one ? "" : `<div class="seg" role="tablist">${PROFILES.map((p) =>
    `<a href="/app?list=${p}" class="${p === list ? "on" : ""}" role="tab" aria-selected="${p === list}">${listNames[p]}</a>`).join("")}</div>`;
  const kindleNow = s.profile === "night" ? "the night clock" : one ? "your tasks" : `the ${listNames[kindleList]} list`;
  const modes = `<div class="seg">${["work", "home", "auto"].map((m) =>
    post(`/do/mode/${m}`, m === "auto" ? "Auto" : listNames[m], s.mode === m ? "on" : "")).join("")}</div>`;
  // calendar links (Google and others); the Mac's Calendar app syncs by itself
  const cals = (await db.get("calendars")) || [];
  const macCal = await db.get("cal_push_mac");
  const calCache = await db.calendarCache();
  const calBox = `<details class="cal"><summary><div><b>Calendars</b><span>The Mac's calendar, and links to others</span></div></summary>
${macCal ? `<div class="line"><div>Mac Calendar app<div class="h">${macCal.events.length} events · synced ${new Date(macCal.at * 1000).toLocaleTimeString("en-GB", { timeZone: db.tz, hour: "2-digit", minute: "2-digit" })}</div></div></div>` : ""}
${cals.map((c, i) => `<div class="line"><div>${esc(c.name)}<div class="h">${c.profile === "both" ? "Both lists" : listNames[c.profile] || title(c.profile)}</div></div>${post(`/do/cal-del/${i}`, "Remove")}</div>`).join("")}
${calCache.errors?.length ? `<p class="status">Couldn't load: ${esc(calCache.errors.join("; "))}</p>` : ""}
<form class="calform" method="post" action="/do/cal-add">${ret}
<p class="status">Add a calendar by its private link (Google Calendar: Settings › your calendar › “Secret address in iCal format”).</p>
<input type="text" name="name" placeholder="Name" maxlength="30" aria-label="Calendar name">
<input type="url" name="url" placeholder="https://… .ics" required aria-label="Calendar link">
<select name="profile" aria-label="Show on"><option value="both">Both lists</option><option value="work">Mac</option><option value="home">${esc(pcLabel)}</option></select>
<div><button class="pill solid">Add calendar</button></div></form></details>`;
  return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)"><meta name="theme-color" content="#000000" media="(prefers-color-scheme: dark)">
<title>${esc(name)} · Desk Buddy</title><style>${APP_CSS}</style></head><body><div class="w">
<header class="top"><div class="r"><h1>${esc(name)}</h1>${listSwitch}</div>
<div class="status">${top.length ? `${done} of ${top.length} done` : "Nothing yet"}${list === kindleList && s.profile !== "night" ? " · on the Kindle now" : ""}</div></header>
${msg ? `<div class="box"><b>Heart message on the Kindle</b><p>“${esc(msg.text)}” is showing instead of your tasks.</p>${post("/do/clear-message", "Back to tasks", "pill solid")}</div>` : ""}
<form class="add" method="post" action="/do/add">${ret}<input type="hidden" name="profile" value="${list}">
<input type="text" name="text" placeholder="Add a task" required maxlength="120" aria-label="New task"><button class="plus" aria-label="Add task">${icon.plus}</button></form>
<div>${top.map((x) => row(x, false)).join("") || '<p class="empty">No tasks. Add one above.</p>'}</div>
${top.length ? `<div class="progress"><div class="bar"><i style="width:${Math.round((done / top.length) * 100)}%"></i></div>${done} / ${top.length} done</div>` : ""}
<div class="label">On the Kindle</div>
<div class="kindle"><div>Showing ${kindleNow}<div class="h">${single ? "" : esc(s.mode === "auto" ? "Auto: follows the computer you're using" : "Picked by hand, back to Auto at 4am")}${seen ? (single ? "" : " · ") + esc(seen) : ""}</div></div>${one ? "" : modes}</div>
${single ? `<div class="line"><div>Searching for other devices…<div class="h">Install Desk Buddy on another computer to connect it.</div></div></div>` : ""}
<div class="label">More</div>
${calBox}
</div>
<script>
// keep the place on the page after a tap (each tap reloads it)
addEventListener("submit", () => sessionStorage.y = scrollY);
if (sessionStorage.y) { scrollTo(0, +sessionStorage.y); delete sessionStorage.y; }
</script></body></html>`;
}

// "2 min ago", for the phone page
const ago = (sec) => (sec < 90 ? "just now" : sec < 3600 ? `${Math.round(sec / 60)} min ago` : sec < 86400 ? `${Math.round(sec / 3600)} h ago` : `${Math.round(sec / 86400)} days ago`);

// The music playing most recently on any computer (reported in the last 10 minutes).
function musicNow(kv) {
  const all = ["mac", "pc"].map((m) => kv[`music_${m}`] && { ...kv[`music_${m}`], machine: m }).filter(Boolean);
  // Playing on both (Spotify shows the same song as playing on every device it's signed in
  // on): the computer touched last wins, so the Kindle doesn't flip between them every few
  // seconds (2026-10-11: the PC's output list kept appearing and vanishing).
  const hb = kv.heartbeats || {};
  const used = (m) => (hb[m.machine] ? hb[m.machine].at - (hb[m.machine].idle || 0) : 0);
  const fresh = all.filter((m) => now() - m.at < 600).sort((a, b) => (b.playing - a.playing) || (used(b) - used(a)) || (b.at - a.at));
  if (!fresh[0]) return null;
  // the cover only counts when it's this song's (a failed or late send must not leave the
  // previous song's picture next to the new title)
  const m = fresh[0], key = `${m.title}|${m.artist}|${m.album}`.slice(0, 200);
  return { ...m, art_key: kv.music_art_key === key ? key : "" };
}

// ---------- helpers ----------

function cookie(request, name) {
  const m = (request.headers.get("cookie") || "").match(new RegExp(`(?:^|;\\s*)${name}=([^;]*)`));
  return m ? m[1] : null;
}

async function body(request) {
  try {
    return (await request.json()) || {};
  } catch {
    throw new HttpError(400, "Send JSON.");
  }
}

const json = (v, status = 200) => new Response(JSON.stringify(v), { status, headers: { "content-type": "application/json" } });
const html = (s, status = 200) => new Response(s, { status, headers: { "content-type": "text/html; charset=utf-8" } });
const text = (s, status) => new Response(s, { status, headers: { "content-type": "text/plain; charset=utf-8" } });
const redirect = (to) => new Response(null, { status: 303, headers: { location: to } });

// ---------- pairing a Kindle with a local server (local mode only) ----------
// A Kindle without the key asks to connect (POST /api/pair → {id, code}); the Mac helper
// sees the request (GET /api/pair/pending, with the key) and asks the user, who checks the
// code matches the Kindle's and clicks Allow (POST /api/pair/<id> {allow}). The Kindle,
// asking GET /api/pair/<id>, then gets the key once. Requests live in memory for 5 minutes.
const pairs = new Map();

function pairing(path, method, authed, key, b) {
  const t = Date.now() / 1000;
  for (const [id, p] of pairs) if (t - p.at > 300) pairs.delete(id);
  if (path === "/api/pair" && method === "POST") {
    if ([...pairs.values()].filter((p) => p.state === "waiting").length >= 5) return text("Too many requests waiting.", 429);
    const id = crypto.randomUUID();
    const code = String(1000 + Math.floor(Math.random() * 9000));
    pairs.set(id, { id, code, name: String(b.name || "Kindle").slice(0, 40), at: t, state: "waiting" });
    return json({ id, code });
  }
  if (path === "/api/pair/pending" && method === "GET") {
    if (!authed) return null; // the usual "needs the key" answer
    return json([...pairs.values()].filter((p) => p.state === "waiting").map(({ id, code, name, at }) => ({ id, code, name, at })));
  }
  const m = path.match(/^\/api\/pair\/([0-9a-f-]{36})$/);
  if (!m) return null;
  const p = pairs.get(m[1]);
  if (method === "POST") {
    if (!authed) return null;
    if (!p || p.state !== "waiting") return text("That request has gone.", 404);
    p.state = b.allow ? "ok" : "denied";
    return json({ ok: true });
  }
  if (method === "GET") {
    if (!p) return json({ state: "expired" });
    if (p.state === "ok") {
      pairs.delete(p.id); // the key is handed over once
      return json({ state: "ok", key });
    }
    return json({ state: p.state });
  }
  return null;
}
