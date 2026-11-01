(() => {
  const REPO = "jaba0x/SwitchDesk";
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // ---------- nav border once scrolled ----------
  const nav = document.querySelector(".nav");
  const onScroll = () => nav.classList.toggle("scrolled", scrollY > 8);
  addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // ---------- port LED field behind the hero ----------
  const cv = document.querySelector(".ports");
  if (cv) {
    const ctx = cv.getContext("2d");
    let cells = [], w = 0, h = 0, dpr = 1;
    const COLORS = { off: [42, 49, 61], up: [76, 195, 138], warn: [232, 176, 74] };
    const layout = () => {
      dpr = Math.min(devicePixelRatio || 1, 2);
      w = cv.clientWidth; h = cv.clientHeight;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      const pw = 14, ph = 10, gx = 7, gy = 9, block = 6; // ports come in groups of 6, like a faceplate
      cells = [];
      for (let y = 18, row = 0; y < h; y += ph + gy, row++) {
        let x = 10 - ((row % 2) * 11);
        for (let col = 0; x < w; col++) {
          if (col && col % block === 0) x += 16;
          const r = Math.random();
          const state = r < 0.22 ? "up" : r < 0.23 ? "warn" : "off";
          cells.push({ x, y, state, phase: Math.random() * Math.PI * 2, speed: 0.6 + Math.random() * 1.6 });
          x += pw + gx;
        }
      }
      cells.pw = pw; cells.ph = ph;
    };
    const draw = (t) => {
      ctx.clearRect(0, 0, w, h);
      for (const c of cells) {
        const [r, g, b] = COLORS[c.state];
        let a = c.state === "off" ? 0.32 : 0.18 + 0.42 * (0.5 + 0.5 * Math.sin(c.phase + t * 0.001 * c.speed));
        if (c.state === "warn") a = 0.25 + 0.55 * (0.5 + 0.5 * Math.sin(c.phase + t * 0.004));
        ctx.fillStyle = `rgba(${r},${g},${b},${a})`;
        ctx.beginPath();
        ctx.roundRect ? ctx.roundRect(c.x, c.y, cells.pw, cells.ph, 2) : ctx.rect(c.x, c.y, cells.pw, cells.ph);
        ctx.fill();
      }
    };
    // now and then a port links up or drops, like a real switch
    const flip = () => {
      for (let i = 0; i < 3; i++) {
        const c = cells[(Math.random() * cells.length) | 0];
        if (c && c.state !== "warn") c.state = c.state === "up" ? "off" : "up";
      }
    };
    layout();
    let visible = true;
    new IntersectionObserver(([e]) => { visible = e.isIntersecting; }).observe(cv);
    if (reduce) {
      draw(0);
    } else {
      let lastFlip = 0;
      const loop = (t) => {
        if (visible) {
          if (t - lastFlip > 900) { flip(); lastFlip = t; }
          draw(t);
        }
        requestAnimationFrame(loop);
      };
      requestAnimationFrame(loop);
    }
    let rt;
    addEventListener("resize", () => { clearTimeout(rt); rt = setTimeout(() => { layout(); draw(performance.now()); }, 150); });
  }

  // ---------- reveal on scroll ----------
  const els = document.querySelectorAll(".feature > *, .divider");
  if (!reduce && "IntersectionObserver" in window) {
    const io = new IntersectionObserver((entries) => {
      for (const e of entries) if (e.isIntersecting) { e.target.classList.add("in"); io.unobserve(e.target); }
    }, { rootMargin: "0px 0px -8% 0px" });
    els.forEach((el) => { el.classList.add("reveal"); io.observe(el); });
  }

  // ---------- hero: the main screens in turn ----------
  const slides = document.querySelector("[data-slides]");
  if (slides) {
    const label = document.querySelector(".stage .titlebar span");
    const screens = [
      ["switches.webp", "Switches", "Switch list with search and bulk actions"],
      ["switch-overview.webp", "Switch", "Switch page with problems, system facts and spanning tree"],
      ["map.webp", "Network map", "Network map laid out from the spanning-tree root"],
      ["vlan-review.webp", "VLANs", "Review screen with the exact commands before they're sent"],
      ["firmware-update.webp", "Firmware", "Firmware update to the spare bank, one switch at a time"],
    ];
    for (const [file, title, alt] of screens) {
      const img = new Image(1440, 880);
      img.alt = alt; img.dataset.title = title; img.loading = "lazy"; img.decoding = "async";
      img.src = "img/" + file;
      slides.appendChild(img);
    }
    const imgs = [...slides.querySelectorAll("img")];
    const dots = document.createElement("span");
    dots.className = "dots";
    dots.innerHTML = imgs.map((im, i) => `<button type="button" aria-label="Show ${im.dataset.title}"${i ? "" : ' class="on"'}></button>`).join("");
    document.querySelector(".stage .titlebar").appendChild(dots);
    let cur = 0, timer = 0, hover = false;
    const show = (n) => {
      imgs[cur].classList.remove("on"); dots.children[cur].classList.remove("on");
      cur = (n + imgs.length) % imgs.length;
      // restart the slow zoom on the new screen
      imgs[cur].style.animation = "none"; void imgs[cur].offsetWidth; imgs[cur].style.animation = "";
      imgs[cur].classList.add("on"); dots.children[cur].classList.add("on");
      if (label) {
        label.style.opacity = 0;
        setTimeout(() => { label.textContent = `SwitchDesk — ${imgs[cur].dataset.title}`; label.style.opacity = 1; }, 250);
      }
    };
    const run = () => { clearInterval(timer); if (!reduce) timer = setInterval(() => { if (!hover && !document.hidden) show(cur + 1); }, 4500); };
    [...dots.children].forEach((b, i) => b.addEventListener("click", () => { show(i); run(); }));
    slides.addEventListener("mouseenter", () => { hover = true; });
    slides.addEventListener("mouseleave", () => { hover = false; });
    run();
  }

  // ---------- screenshots tilt back until they reach the middle of the screen ----------
  if (!reduce) {
    const tilted = [document.querySelector(".stage .window"), ...document.querySelectorAll(".shot")].filter(Boolean);
    let ticking = false;
    const tilt = () => {
      ticking = false;
      const vh = innerHeight;
      for (const el of tilted) {
        const r = el.getBoundingClientRect();
        if (r.bottom < 0 || r.top > vh) continue;
        // 1 when its top is at the bottom of the screen, 0 once it's a third of the way up
        const t = Math.min(1, Math.max(0, (r.top - vh * 0.33) / (vh * 0.67)));
        el.style.setProperty("--tilt", t.toFixed(3));
      }
    };
    addEventListener("scroll", () => { if (!ticking) { ticking = true; requestAnimationFrame(tilt); } }, { passive: true });
    addEventListener("resize", tilt);
    tilt();
  }

  // ---------- highlight the visitor's platform ----------
  const ua = navigator.userAgent;
  const mine = /Windows/i.test(ua) ? "win" : /Mac OS X|Macintosh/i.test(ua) ? "mac" : "";
  if (mine) {
    document.querySelector(`.plat[data-plat="${mine}"]`)?.classList.add("mine");
  }

  // ---------- latest release: version, sizes, direct links ----------
  const mb = (n) => (n / 1048576).toFixed(1) + " MB";
  fetch(`https://api.github.com/repos/${REPO}/releases/latest`, { headers: { Accept: "application/vnd.github+json" } })
    .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
    .then((rel) => {
      const ver = (rel.tag_name || "").replace(/^v/, "");
      const date = rel.published_at ? new Date(rel.published_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }) : "";
      document.querySelectorAll("[data-ver]").forEach((el) => { el.textContent = ver ? `${ver}${date ? " · " + date : ""}` : "Latest"; });
      const pick = { mac: /macos/i, win: /windows/i };
      for (const [k, re] of Object.entries(pick)) {
        const a = (rel.assets || []).find((x) => re.test(x.name));
        if (!a) continue;
        document.querySelectorAll(`[data-dl="${k}"]`).forEach((el) => { el.href = a.browser_download_url; });
        document.querySelectorAll(`[data-size="${k}"]`).forEach((el) => { el.textContent = mb(a.size); });
      }
    })
    .catch(() => {});

  fetch(`https://api.github.com/repos/${REPO}`)
    .then((r) => (r.ok ? r.json() : Promise.reject()))
    .then((repo) => {
      const n = repo.stargazers_count;
      const el = document.querySelector("[data-stars]");
      if (el && typeof n === "number" && n > 0) el.textContent = "★ " + (n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n));
    })
    .catch(() => {});
})();
