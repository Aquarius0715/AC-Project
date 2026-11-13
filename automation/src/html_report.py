"""Offline HTML reports: no fetch, server, CDN or runtime dependencies."""

import html
import json
import os
from pathlib import Path

from .dataset import inside_automation, write_json

CSS = """
:root{color-scheme:light;--ink:#17324d;--line:#d7e1eb;--blue:#165dcc}*{box-sizing:border-box}
body{margin:0;background:#f3f6fa;color:var(--ink);font:14px/1.7 system-ui,"Noto Sans CJK JP","Yu Gothic",Meiryo,sans-serif}header{background:#102f50;color:white;padding:28px max(24px,calc((100vw - 1400px)/2));border-bottom:5px solid #53c8aa}h1{font-size:26px;margin:0}h2{font-size:20px}h3{font-size:16px}header p{color:#d1e4f3}main{max-width:1448px;margin:auto;padding:24px}a{color:var(--blue)}header a{color:#aee5ff}
button,select,input{font:inherit;padding:7px 10px;border:1px solid #bccbd9;border-radius:6px;background:white;color:var(--ink)}button{cursor:pointer}button:focus-visible,a:focus-visible,input:focus-visible{outline:3px solid #ebad24;outline-offset:2px}.cards{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:24px}.card{background:white;border:1px solid var(--line);border-radius:9px;padding:16px 22px;min-width:160px}.card strong{display:block;font-size:28px}.muted{color:#5c7084}.panel{background:white;border:1px solid var(--line);border-radius:9px;padding:20px;margin-bottom:20px;overflow:auto}table{width:100%;border-collapse:collapse;font-size:13px}th,td{text-align:left;padding:10px;border-bottom:1px solid var(--line);vertical-align:top}th{background:#edf3f9;white-space:nowrap}tr:hover td{background:#f8fbff}.layout>*{min-width:0}.layout{display:grid;grid-template-columns:280px minmax(0,1fr);gap:20px}.timeline{max-height:78vh;overflow:auto}.day{width:100%;text-align:left;margin-bottom:5px;border-left:5px solid #c9d5e2}.day.first,.day.reappeared{border-left-color:#139b72}.day.changed{border-left-color:#e29e13}.day.ended{border-left-color:#bd5c56}.day.response{border-right:5px solid #8859bc}.day.selected{background:#e5f0ff;border-color:#165dcc}.badge{display:inline-block;padding:2px 7px;border-radius:4px;background:#eaf0f5;font-size:12px;margin-right:5px}.badge.proposed{background:#d9f3e9;color:#086445}.badge.suppressed{background:#eee2fa;color:#693a88}.notice{border-left:4px solid #53c8aa;padding:12px 16px;background:#f0faf6}pre{white-space:pre-wrap;word-break:break-word;background:#f3f6fa;padding:12px;font:12px/1.6 monospace;max-height:450px;overflow:auto}details{margin:10px 0}summary{cursor:pointer;font-weight:600}.controls{display:flex;gap:8px;align-items:center;flex-wrap:wrap}@media(max-width:850px){.layout{grid-template-columns:1fr}.timeline{max-height:260px}main{padding:12px}header{padding:20px}}
"""
LABELS = dict(
    first="初回",
    continued="継続",
    changed="内容変更",
    reappeared="再出現",
    ended="終了",
    proposed="提案",
    unchanged="変更不要",
    insufficient="根拠不足",
    suppressed="見送り済み",
    approved="承認済み",
    merged="根拠を統合",
    unavailable="比較データなし",
    rejected="条件不一致",
    start="開始変更",
    start_review="開始見直し",
    add_schedule="初回予定",
    temperature="暑い日の温度",
    precool="予冷",
    declined="見送り",
    edited="修正して承認",
    no_response="無反応",
)
COMMON = """
const labels=LABELS; const label=x=>labels[x]||x;
const reason=x=>({'presence_unknown':'在宅情報が不明','someone_home':'在宅者を確認','exception_excluded':'旅行・例外・予定差などを除外','absence_not_repeated':'不在の繰り返しが不足'}[x]||x.split(':').map(label).join('：'));
const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n};
function table(parent,headers,rows){const t=el('table');const h=el('tr');headers.forEach(x=>h.append(el('th',x)));t.append(h);rows.forEach(row=>{const r=el('tr');row.forEach(c=>r.append(el('td',typeof c==='object'?JSON.stringify(c):String(c??'不明'))));t.append(r)});parent.append(t)}
function detail(parent,title,value){const d=el('details');d.append(el('summary',title),el('pre',JSON.stringify(value,null,2)));parent.append(d)}
""".replace("LABELS", json.dumps(LABELS, ensure_ascii=False))


def page(title, body, payload, script):
    embedded = json.dumps(payload, ensure_ascii=False, default=str).replace(
        "<", "\\u003c"
    )
    return f'<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{html.escape(title)}</title><style>{CSS}</style></head><body>{body}<script id="data" type="application/json">{embedded}</script><script>{COMMON}\nconst data=JSON.parse(document.getElementById("data").textContent);\n{script}</script></body></html>'


def customer_html(report, path):
    path = inside_automation(path)
    href = html.escape(os.path.relpath(report["input_path"], path.parent), quote=True)
    hid = html.escape(report["household_id"])
    body = f'''<header><a href="../index.html">← 全体一覧</a><h1>{hid} の提案履歴</h1><p>仮データ / 承認型・未適用 / 対象日の翌日12:00（Asia/Tokyo）に判定</p><a href="{href}">入力JSONを開く</a> · <a href="{hid}.json">評価JSONを開く</a></header><main><div class="cards"><div class="card">新規提案<strong>{report["new_proposal_count"]}</strong></div><div class="card">判定回数<strong>{len(report["days"])}</strong></div><div class="card">最初の提案<strong style="font-size:18px">{(report["first_proposal_at"] or "なし")[:10]}</strong></div></div><div class="layout"><aside class="panel"><h2>日付順のタイムライン</h2><label>表示 <select id="filter"><option value="all">すべて</option><option value="new">新規・変更・再出現</option><option value="response">承認・見送りあり</option><option value="none">提案0件</option><option value="ended">終了した日</option></select></label><div id="timeline" class="timeline"></div></aside><section><div class="panel"><div class="controls"><button id="prev">前の日</button><button id="next">次の日</button><label>判定日 <select id="date"></select></label></div><h2 id="day-title"></h2><p id="day-meta" class="muted"></p><div id="events"></div></div><div id="detail"></div></section></div></main>'''
    script = """
let selected=0;const timeline=document.getElementById('timeline'),chooser=document.getElementById('date');data.days.forEach((d,i)=>{let o=el('option',d.decision_at.slice(0,10));o.value=i;chooser.append(o)});
function renderTimeline(){timeline.replaceChildren();let f=document.getElementById('filter').value;data.days.forEach((d,i)=>{if(f==='new'&&!d.new_count||f==='response'&&!d.responses.length||f==='none'&&d.active_count||f==='ended'&&!d.events.some(e=>e.event==='ended'))return;let k=d.events.find(e=>e.is_new)?.event||d.events.find(e=>e.event==='ended')?.event||'';let b=el('button',d.decision_at.slice(5,10)+'　新規 '+d.new_count+' / 有効 '+d.active_count+(k?' / '+label(k):'')+(d.responses.length?' / 反応':''),'day '+k+(d.responses.length?' response':'')+(i===selected?' selected':''));b.setAttribute('aria-pressed',String(i===selected));b.onclick=()=>show(i);timeline.append(b)});timeline.querySelector('.selected')?.scrollIntoView({block:'nearest'})}
function show(i){selected=Math.max(0,Math.min(data.days.length-1,i));chooser.value=selected;renderTimeline();let d=data.days[selected];document.getElementById('day-title').textContent=d.decision_at.slice(0,10)+' の判定';document.getElementById('day-meta').textContent='対象日 '+d.target_date+' / 判定 '+d.decision_at+' / 新規 '+d.new_count+'件 / 有効 '+d.active_count+'件';let events=document.getElementById('events');events.replaceChildren();if(!d.events.length)events.append(el('p','有効な提案はありません。下記の判定理由を参照してください。'));d.events.forEach(e=>{let p=el('p');p.append(el('span',label(e.event),'badge '+e.event),el('span',e.proposal_id+' / '+label(e.kind)+' / '+e.device_id+(e.target_date?' / 対象日 '+e.target_date:'')+' / '+(e.weekday===null?'共通':'月火水木金土日'[e.weekday]+'曜')+' / '+JSON.stringify(e.values)));events.append(p)});d.responses.forEach(r=>events.append(el('p','ユーザーの反応: '+label(r.response)+' / 実際の日時 '+r.responded_at+' / 選択 '+JSON.stringify(r.selected),'notice')));
let target=document.getElementById('detail');target.replaceChildren();d.devices.forEach(device=>{let p=el('article',undefined,'panel');p.append(el('h2',device.device_id),el('h3','判定時点の現行予定'));table(p,['曜日','開始','終了','温度'],device.current_schedules.map(s=>['月火水木金土日'[s.weekday],s.start,s.end,s.temperature_c+'℃']));p.append(el('h3','各判定と支持・根拠'));device.decisions.forEach(item=>{let b=el('details');b.open=['proposed','suppressed'].includes(item.status);let s=el('summary');s.append(el('span',label(item.status),'badge '+item.status),el('span',label(item.kind)+' / '+(item.weekday===null?'共通':'月火水木金土日'[item.weekday]+'曜')+' / 支持 '+item.support_count+'日'));b.append(s,el('p',item.reason),el('p','内容 '+JSON.stringify(item.values)));let ul=el('ul');[...item.evidence,...item.judgment_influence].forEach(x=>ul.append(el('li',x)));b.append(ul);if(item.comparisons)table(b,['根拠日','当日の開始','世帯','帰宅確認','除外'],item.comparisons.map(r=>[r.date,r.scheduled_at.slice(11,16),r.state,r.confirmed_return?.confirmed_at||'未確認',r.excluded_reason||'なし']));if(item.estimate)detail(b,'候補別費用',item.estimate);detail(b,'根拠ID・状態の詳細',item);p.append(b)});target.append(p)});
let ctx=el('article',undefined,'panel');ctx.append(el('h2','対象日 '+d.target_date+' の入力（判定までに分かった情報）'),el('h3','スマホ由来の在宅・外出'));table(ctx,['住人','観測時刻','受信時刻','情報源','イベント','状態','利用'],d.context.presence.map(o=>[o.resident_id,o.observed_at.slice(11,19),o.received_at,o.source,o.event,o.state,o.sharing_enabled?'有効':'停止']));d.context.devices.forEach(device=>{ctx.append(el('h3',device.device_id+' / 対象日に有効だった予定'));table(ctx,['曜日','開始','終了','温度'],device.observed_schedules.map(s=>['月火水木金土日'[s.weekday],s.start,s.end,s.temperature_c]));ctx.append(el('h3','関連操作と電源状態'));table(ctx,['日時','操作元','操作','温度','電源前→後','受信日時'],device.operations.map(o=>[o.at,o.source,o.action,(o.from_c??'')+' → '+(o.to_c??''),o.power?String(o.power.before)+' → '+String(o.power.after):'未確認',o.received_at]));detail(ctx,'30分電力量・単価・料金',device.intervals);detail(ctx,'天気の実測相当データ',device.weather)});target.append(ctx)}
document.getElementById('filter').onchange=renderTimeline;chooser.onchange=()=>show(Number(chooser.value));document.getElementById('prev').onclick=()=>show(selected-1);document.getElementById('next').onclick=()=>show(selected+1);show(0);
"""
    path.write_text(page(f"{hid} 提案履歴", body, report, script), encoding="utf-8")


def write_report(result, output: Path):
    output = inside_automation(output)
    output.mkdir(parents=True, exist_ok=True)
    folder = output / "customers"
    folder.mkdir(exist_ok=True)
    summaries = []
    for report in result["customers"]:
        write_json(folder / f"{report['household_id']}.json", report)
        customer_html(report, folder / f"{report['household_id']}.html")
        summaries.append({k: v for k, v in report.items() if k != "days"})
    summary = {k: v for k, v in result.items() if k != "customers"}
    summary["customers"] = summaries
    write_json(output / "summary.json", summary)
    return index_html(summary, output)


def index_html(summary, output):
    output = inside_automation(output)
    result = summary
    body = f"""<header><h1>エアコン運転提案の再生レポート</h1><p>仮データによる承認型デモ · 自動制御・予定適用は行いません</p></header><main><div class="cards"><div class="card">世帯<strong>{result["customer_count"]}</strong></div><div class="card">判定回数 / 世帯<strong>{result["day_count"]}</strong></div><div class="card">新規提案（継続を除く）<strong>{result["new_proposal_count"]}</strong></div><div class="card">評価時間<strong>{result["evaluation_seconds"]:.2f}s</strong></div></div><div class="panel"><h2>顧客別の結果</h2><p class="muted">初回・内容変更・再出現を新規として数え、継続と承認・見送りは加算しません。</p><label>顧客を検索 <input id="search" placeholder="H001"></label><div id="list"></div></div><div class="panel"><h2>見送り・変更不要の内訳</h2><p class="muted">機器・曜日・種類・判定日ごとの回数です。</p><div id="reasons"></div><a href="summary.json">集計JSONを開く</a></div></main>"""
    script = """
function render(){let p=document.getElementById('list');p.replaceChildren();let q=document.getElementById('search').value.toLowerCase(),t=el('table'),h=el('tr');['顧客','新規件数','種類','最初の提案日','最後の判定での見送り理由'].forEach(x=>h.append(el('th',x)));t.append(h);data.customers.filter(r=>r.household_id.toLowerCase().includes(q)).forEach(r=>{let row=el('tr'),cell=el('td'),a=el('a',r.household_id);a.href='customers/'+encodeURIComponent(r.household_id)+'.html';cell.append(a);row.append(cell,el('td',r.new_proposal_count),el('td',Object.entries(r.new_by_kind).map(([k,v])=>label(k)+' '+v).join(' / ')||'なし'),el('td',r.first_proposal_at?.slice(0,10)||'なし'),el('td',Object.entries(r.last_reason_counts).map(([k,v])=>reason(k)+': '+v).join(' / ')));t.append(row)});p.append(t)}document.getElementById('search').oninput=render;render();table(document.getElementById('reasons'),['理由','判定回数'],Object.entries(data.decision_day_reason_counts).map(([k,v])=>[reason(k),v]));
"""
    (output / "index.html").write_text(
        page("運転提案の再生レポート", body, summary, script), encoding="utf-8"
    )
    return output / "index.html"
