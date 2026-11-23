# 評測：官方 0.2.14

- 官方 0.2.14：引擎 dev，2026-11-24 01:29 跑，每句 3 次；資料 0.2.14-hoshi.0.1-883ffa43adb2（sha256 0ab52e5f6182）
  - user：神經統計（無文字）：{Calls:28206 Scored:28206 Timeouts:0 Errors:0 Unavailable:0 TotalUS:234982595} 重啟 0 次
  - fullwidth：神經統計（無文字）：{Calls:0 Scored:0 Timeouts:0 Errors:0 Unavailable:0 TotalUS:0} 重啟 0 次

「正確」是打完（不送出）畫面上的文字跟 want 完全一樣；␣ 是半形空白。每鍵耗時是引擎處理那一鍵的時間（含神經選字），不含管道往返；每句跑好幾次時取每一鍵最快的一次。

## 總覽

| 語料 | 句數 | 正確 | 正確率 | 統計選字 | 重跑不同 |
|---|---:|---:|---:|---:|---:|
| asus-mixed（華碩參考） | 255 | 87 | 34.1% | 0 | 0 |
| asus-redundant（華碩參考） | 128 | 40 | 31.2% | 0 | 0 |
| asus-typo（華碩參考） | 1033 | 200 | 19.4% | 0 | 0 |
| mixed-habit1 | 150 | 111 | 74.0% | 0 | 0 |
| mixed-habit2 | 150 | 95 | 63.3% | 0 | 0 |
| spec | 55 | 21 | 38.2% | 0 | 0 |
| zh-habit1（要跟對照一樣） | 200 | 172 | 86.0% | 0 | 0 |
| zh-habit2（要跟對照一樣） | 200 | 185 | 92.5% | 0 | 0 |
| spec-fullwidth | 2 | 0 | 0.0% | 0 | 0 |

## 每鍵耗時（毫秒）

| 語料 | 送進引擎的鍵 | p50 | p95 | p99 | 最大 |
|---|---:|---:|---:|---:|---:|
| asus-mixed | 1395 | 0.0 | 6.4 | 8.3 | 9.5 |
| asus-redundant | 775 | 0.0 | 0.5 | 6.5 | 7.9 |
| asus-typo | 4856 | 0.0 | 5.9 | 6.9 | 27.8 |
| mixed-habit1 | 6853 | 0.0 | 13.1 | 19.4 | 60.6 |
| mixed-habit2 | 6159 | 0.0 | 13.0 | 17.6 | 43.6 |
| spec | 318 | 0.0 | 7.3 | 13.5 | 18.7 |
| zh-habit1 | 11248 | 0.0 | 19.5 | 29.2 | 63.2 |
| zh-habit2 | 8722 | 0.0 | 14.5 | 19.2 | 33.7 |
| spec-fullwidth | 15 | 0.0 | 0.0 | 0.0 | 0.0 |

<details><summary>asus-mixed：錯 168 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| mixed-001 | `eu04` | e驗 | eu04 | b11␣illegal |
| mixed-002 | `du04` | d驗 | du04 | b11␣illegal |
| mixed-003 | `cu04` | c驗 | cu暗 | b11␣illegal |
| mixed-004 | `gu04` | g驗 | gu04 | b11␣illegal |
| mixed-005 | `5u04` | 5驗 | 5堰 | b11␣illegal |
| mixed-006 | `tu04` | t驗 | tu04 | b11␣illegal |
| mixed-007 | `bu04` | b驗 | bu暗 | b11␣illegal |
| mixed-008 | `yu04` | y驗 | yu04 | b11␣illegal |
| mixed-009 | `hu04` | h驗 | hu04 | b11␣illegal |
| mixed-010 | `nu04` | n驗 | nu案 | b11␣illegal |
| mixed-011 | `zu04` | z驗 | zu04 | b11␣illegal |
| mixed-012 | `rj04` | r萬 | rj04 | b11␣illegal |
| mixed-013 | `fj04` | f萬 | fj04 | b11␣illegal |
| mixed-014 | `vj04` | v萬 | vj04 | b11␣illegal |
| mixed-015 | `am04` | am04 | am案 | b11␣illegal |
| mixed-016 | `em4` | e預 | em4 | b11␣illegal |
| mixed-017 | `gm4` | g預 | gm4 | b11␣illegal |
| mixed-018 | `1m4` | 1預 | 1與 | b11␣illegal |
| mixed-019 | `wm4` | w預 | wm4 | b11␣illegal |
| mixed-020 | `eu4` | e意 | eu4 | b11␣illegal |
| mixed-021 | `gu4` | g意 | gu4 | b11␣illegal |
| mixed-022 | `rj4` | r物 | rj4 | b11␣illegal |
| mixed-025 | `vl3` | v襖 | vl3 | b11␣illegal |
| mixed-026 | `r.4` | r嘔 | r.4 | b11␣illegal |
| mixed-029 | `fk4` | f惡 | fk4 | b11␣illegal |
| mixed-031 | `ml4` | m奧 | ml4 | b11␣illegal |
| mixed-032 | `j,4` | j誒 | j,4 | b11␣illegal |
| mixed-033 | `m;4` | m盎 | m;4 | b11␣illegal |
| mixed-034 | `2up4` | 2朄 | 2印 | b11␣illegal |
| mixed-037 | `wj;4` | w望 | wj;4 | b11␣illegal |
| mixed-038 | `gj/4` | g甕 | gj/4 | b11␣illegal |
| mixed-040 | `rm84` | rm84 | rm阿 | b11␣illegal |
| mixed-041 | `sj94` | s外 | sj94 | b11␣illegal |
| mixed-042 | `s9␣` | s唉 | ㄋㄞ | b11␣illegal |
| mixed-043 | `cj9␣` | c歪 | ㄏㄨㄞ | b11␣illegal |
| mixed-044 | `eo4` | e欸 | eo4 | b11␣illegal |
| mixed-045 | `qo3` | 琣 | 昢 | b11␣illegal |
| mixed-048 | `vbn␣` | v斯 | vb斯 | b11␣illegal |
| mixed-052 | `ji3eu04` | 我e驗 | 我eu04 | b11␣illegal |
| mixed-055 | `u04` | 驗 | 燕 | b11␣illegal |
| mixed-056 | `ru84` | 架 | 價 | b11␣illegal |
| mixed-057 | `wu/su3` | 擰 | wu/su3 | b11␣notone |
| mixed-058 | `wu/xu3` | w領 | wu/xu3 | b11␣notone |
| mixed-059 | `wu/cp3` | wu很 | wu/cp3 | b11␣notone |
| mixed-060 | `rupwu0␣` | rup天 | rupwu0␣ | b11␣notone |
| mixed-061 | `cpd9␣` | 開 | cpd欸 | b11␣notone |
| mixed-062 | `gkak7` | g麼 | gkak7 | b11␣notone |
| mixed-063 | `gkfu4` | gk器 | gk企 | b11␣notone |
| mixed-064 | `2u0sl3` | 2鳥 | 2u0sl3 | b11␣notone |
| mixed-065 | `vucj0␣` | 歡 | vucj0␣ | b11␣notone |
| mixed-066 | `g.ru␣` | 糾 | g.ru␣ | b11␣notone |
| mixed-067 | `g.xu␣` | g溜 | g.xu␣ | b11␣notone |
| mixed-068 | `wu0fu4` | w歉 | wu0企 | b11␣notone |
| mixed-069 | `w8a83` | w8碼 | w8嗎 | b11␣notone |
| mixed-070 | `x8a83` | x8碼 | x8馬 | b11␣notone |
| mixed-071 | `c8a83` | c8碼 | c8嗎 | b11␣notone |
| mixed-072 | `clal3` | c卯 | cl卯 | b11␣notone |
| mixed-073 | `jiap7` | j們 | ji們 | b11␣notone |
| mixed-074 | `jisu3` | jis以 | ji你 | b11␣notone |
| mixed-075 | `suul4` | 尿 | su要 | b11␣notone |
| mixed-076 | `ji3rupwu0␣cp3d9␣vup␣` | 我rup天很開心 | 我rupwu0␣很開心 | b11␣notone |
| mixed-077 | `ji3rup␣wu0cp3d9␣vup␣` | 我金wu0很開心 | 我今wu0很開心 | b11␣notone |
| mixed-078 | `1jul4` | 鰾 | 1jul4 | b11␣notone |
| mixed-079 | `xu/su3` | 擰 | xu/su3 | b11␣notone |
| mixed-080 | `au/su3` | 擰 | au/su3 | b11␣notone |
| mixed-081 | `3nm54t/6` | ˇnm製程 | 3nm54成 | b12␣alnum |
| mixed-082 | `4ku/6aj4` | ˋk螢幕 | 4ku/6木 | b12␣alnum |
| mixed-083 | `5gg.3ru␣` | 5手機 | 5gg.3機 | b12␣alnum |
| mixed-084 | `a453` | a453 | a4只 | b12␣alnum |
| mixed-085 | `100m06` | 100圓 | 100元 | b12␣alnum |
| mixed-086 | `20wu0␣` | 20天 | 20wu0␣ | b12␣alnum |
| mixed-087 | `$20m06` | $20圓 | $20元 | b12␣alnum |
| mixed-088 | `3ek4` | ˇ各 | 3個 | b12␣alnum |
| mixed-089 | `102u03` | 10點 | 102演 | b12␣alnum |
| mixed-090 | `3dxu,4up4` | ˇ列印 | 3dx夜印 | b12␣alnum |
| mixed-091 | `mp32;3` | 允擋 | 允檔 | b12␣alnum |
| mixed-094 | `2u43au/6` | 第ˇ明 | 第3名 | b12␣alnum |
| mixed-096 | `ji32wu0␣` | 我2天 | 我2wu0␣ | b12␣alnum |
| mixed-097 | `ji3100m06` | 我100圓 | 我100元 | b12␣alnum |
| mixed-098 | `usbvu04` | us現 | usbvu04 | b12␣alnum |
| mixed-099 | `wifiau4a83` | wifi密碼 | wifiau4嗎 | b12␣alnum |
| mixed-100 | `100␣m06` | 10安源 | 100␣元 | b12␣alnum |
| mixed-102 | `5wu0␣` | 5天 | 5wu0␣ | b12␣alnum |
| mixed-103 | `1wu0␣` | 1天 | 1wu0␣ | b12␣alnum |
| mixed-104 | `12wu0␣` | 12天 | 12wu0␣ | b12␣alnum |
| mixed-105 | `365wu0␣` | ˇ65天 | 365wu0␣ | b12␣alnum |
| mixed-106 | `Hellosu3cl3` | Hello妳好 | Hello你好 | b12␣upper |
| mixed-111 | `Tvm4` | T續 | T恤 | b12␣upper |
| mixed-114 | `ji3Asu3` | 我As以 | 我A你 | b12␣upper |
| mixed-116 | `USBvu04` | USB現 | USB線 | b12␣upper |
| mixed-120 | `Gk6ak7` | G額麼 | Gk6麼 | b12␣upper |
| mixed-122 | `PDF2;3` | PDF擋 | PDF檔 | b12␣upper |
| mixed-127 | `ji3.␣su3` | 我ㄡ你 | 我歐你 | b13␣sym |
| mixed-128 | `ji3.su3` | 我.你 | 我扭 | b13␣sym |
| mixed-135 | `ji3.␣` | 我ㄡ | 我歐 | b13␣sym |
| mixed-142 | `ji3,4` | 我誒 | 我,4 | b13␣sym |
| mixed-143 | `ji3.3` | 我嘔 | 我偶 | b13␣sym |
| mixed-146 | `,4` | 誒 | ,4 | b13␣sym |
| mixed-147 | `.3` | 嘔 | 偶 | b13␣sym |
| mixed-148 | `ru,4` | 界 | 屆 | b13␣sym |
| mixed-160 | `3.14` | ˇ.14 | 3.14 | b13␣sym |
| mixed-165 | `1.5wu0␣` | 1.5天 | 1.5wu0␣ | b13␣sym |
| mixed-166 | `ji31.5wu0␣` | 我1.5天 | 我1.5wu0␣ | b13␣sym |
| mixed-167 | `2.5␣` | 2州 | 2.5␣ | b13␣sym |
| mixed-168 | `xupwu0␣` | xup天 | xupwu0␣ | b14␣notone |
| mixed-169 | `xupwul4` | xup跳 | xupw要 | b14␣notone |
| mixed-171 | `xulwu0␣` | xul天 | xulwu0␣ | b14␣notone |
| mixed-172 | `xulwul4` | x跳 | xulw要 | b14␣notone |
| mixed-173 | `xulwu6` | x條 | xul提 | b14␣notone |
| mixed-176 | `xu0wu6` | x填 | xu0提 | b14␣notone |
| mixed-177 | `xu/wu0␣` | xu/天 | xu/wu0␣ | b14␣notone |
| mixed-178 | `xu/wul4` | x跳 | xu/wul4 | b14␣notone |
| mixed-179 | `xu/wu6` | x停 | xu/wu6 | b14␣notone |
| mixed-180 | `xuwu0␣` | x天 | xuwu0␣ | b14␣notone |
| mixed-181 | `xuwul4` | x跳 | xuw要 | b14␣notone |
| mixed-182 | `xuwu6` | x提 | xu提 | b14␣notone |
| mixed-183 | `qupwu0␣` | qup天 | qupwu0␣ | b14␣notone |
| mixed-184 | `qupwul4` | qup跳 | qupw要 | b14␣notone |
| mixed-186 | `qulwu0␣` | qul天 | qulwu0␣ | b14␣notone |
| mixed-187 | `qulwul4` | 跳 | qulw要 | b14␣notone |
| mixed-188 | `qulwu6` | 條 | qul提 | b14␣notone |
| mixed-191 | `qu0wu6` | 填 | qu0提 | b14␣notone |
| mixed-192 | `qu/wu0␣` | qu/天 | qu/wu0␣ | b14␣notone |
| mixed-193 | `qu/wul4` | 跳 | qu/wul4 | b14␣notone |
| mixed-194 | `qu/wu6` | 停 | qu/wu6 | b14␣notone |
| mixed-195 | `quwu0␣` | 天 | quwu0␣ | b14␣notone |
| mixed-196 | `quwul4` | 跳 | quw要 | b14␣notone |
| mixed-197 | `quwu6` | 提 | qu提 | b14␣notone |
| mixed-198 | `rupwu0␣` | rup天 | rupwu0␣ | b14␣notone |
| mixed-199 | `fupwu0␣` | fup天 | fupwu0␣ | b14␣notone |
| mixed-200 | `vupwu0␣` | vup天 | vupwu0␣ | b14␣notone |
| mixed-203 | `wu/cp3` | wu很 | wu/cp3 | b14␣notone |
| mixed-204 | `wu.cp3` | wu很 | wu.cp3 | b14␣notone |
| mixed-205 | `wulcp3` | wu很 | wul很 | b14␣notone |
| mixed-207 | `xu/cp3` | xu很 | xu/cp3 | b14␣notone |
| mixed-208 | `r8␣` | r啊 | r8␣ | b14␣illegal |
| mixed-209 | `f0␣` | f安 | f0␣ | b14␣illegal |
| mixed-210 | `z9␣` | z唉 | z9␣ | b14␣illegal |
| mixed-211 | `m8␣` | m啊 | m8␣ | b14␣illegal |
| mixed-214 | `.␣` | ㄡ | 歐 | b14␣sym |
| mixed-215 | `ji3.␣` | 我ㄡ | 我歐 | b14␣sym |
| mixed-216 | `cl3.␣` | 好ㄡ | 好歐 | b14␣sym |
| mixed-217 | `cl3.␣su3` | 好ㄡ你 | 好歐你 | b14␣sym |
| mixed-218 | `ji3;␣` | 我ㄤ | 我骯 | b14␣sym |
| mixed-219 | `ji3/␣` | 我/␣ | 我鞥 | b14␣sym |
| mixed-221 | `ji3..␣` | 我.ㄡ | 我..␣ | b14␣sym |
| mixed-222 | `hello.␣` | helloㄡ | hello.␣ | b14␣sym |
| mixed-223 | `hello.␣world` | helloㄡworld | hello.␣world | b14␣sym |
| mixed-224 | `1.␣` | 1ㄡ | 1.␣ | b14␣sym |
| mixed-225 | `ji3.g3` | 我.史 | 我手 | b14␣sym |
| mixed-226 | `ji3;g4` | 我;是 | 我上 | b14␣sym |
| mixed-231 | `hello.su3` | hello紐 | hello.你 | b14␣sym |
| mixed-232 | `3` | ˇ | 3 | b14␣alnum |
| mixed-233 | `3␣` | ˇ␣ | 3␣ | b14␣alnum |
| mixed-234 | `4␣` | ˋ␣ | 4␣ | b14␣alnum |
| mixed-237 | `3wu0␣` | ˇ天 | 3天 | b14␣alnum |
| mixed-238 | `30wu0␣` | ˇ0天 | 30wu0␣ | b14␣alnum |
| mixed-239 | `13wu0␣` | 13天 | 13wu0␣ | b14␣alnum |
| mixed-240 | `3g` | ˇg | 3g | b14␣alnum |
| mixed-241 | `4g` | ˋg | 4g | b14␣alnum |
| mixed-243 | `4k` | ˋk | 4k | b14␣alnum |
| mixed-244 | `3d` | ˇd | 3d | b14␣alnum |
| mixed-245 | `ji34k` | 我ˋk | 我4k | b14␣alnum |
| mixed-248 | `Qwu/6` | Q停 | Qw營 | b14␣upper |
| mixed-249 | `Hkg6` | H什 | Hkg6 | b14␣upper |
| mixed-250 | `Swu0␣` | S天 | Swu0␣ | b14␣upper |
| mixed-251 | `Wu0␣` | W淹 | Wu0␣ | b14␣upper |
| mixed-252 | `ji3Qwu/6` | 我Q停 | 我Qw營 | b14␣upper |

</details>

<details><summary>asus-redundant：錯 88 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| red-002 | `wuuuuuuuuuuuuuuuu/6` | 停 | wuuuuuuuuuuuuuuu營 |  |
| red-003 | `wu////////6` | 停 | wu////////6 |  |
| red-004 | `wuuuuuuuuuu////////6` | 停 | wuuuuuuuuuu////////6 |  |
| red-005 | `wuu/6` | 停 | wu營 |  |
| red-006 | `wu//6` | 停 | wu//6 |  |
| red-007 | `wwwwu/6` | 停 | www停 |  |
| red-008 | `wwwwuuuu////6` | 停 | wwwwuuuu////6 |  |
| red-009 | `cjjjjo6` | 回 | cjjj為 |  |
| red-010 | `cjoooo6` | 回 | cjoooo6 |  |
| red-011 | `xuu.4` | 六 | xu又 |  |
| red-012 | `xu..4` | 六 | xu..4 |  |
| red-013 | `ruuuu84` | 架 | ruuuu84 |  |
| red-014 | `ru8884` | 架 | ru8884 |  |
| red-015 | `rrru84` | 架 | rr駕 |  |
| red-016 | `b006` | 然 | b006 |  |
| red-017 | `su;;6` | 娘 | su;;6 |  |
| red-018 | `wu//␣` | 聽 | wu//␣ |  |
| red-019 | `wuuuu/␣` | 聽 | wuuuu/␣ |  |
| red-020 | `cjjjo␣` | 灰 | cjjjo␣ |  |
| red-029 | `hellowuuuu/6` | hello停 | hellowuuu營 |  |
| red-030 | `hellowu//6` | hello停 | hellowu//6 |  |
| red-031 | `ji3wuuuu/6` | 我停 | 我wuuu營 |  |
| red-032 | `ji3wu//6` | 我停 | 我wu//6 |  |
| red-033 | `ji3wuuuu/6su3` | 我停你 | 我wuuu贏你 |  |
| red-034 | `gck6` | g合 | gck6 |  |
| red-035 | `ggggk6` | 什 | ggg什 |  |
| red-037 | `ji33` | 我ˇ | 我3 |  |
| red-038 | `--4` | -二 | --4 |  |
| red-039 | `wuwu/6` | 停 | wu停 |  |
| red-040 | `wu/u/6` | 停 | wu/u/6 |  |
| red-042 | `w004` | 探 | w004 |  |
| red-044 | `w006` | 潭 | w006 |  |
| red-049 | `s994` | 奈 | s994 |  |
| red-050 | `ru884` | 架 | ru884 |  |
| red-052 | `c004` | 瀚 | c004 |  |
| red-054 | `a884` | 罵 | a884 |  |
| red-057 | `ruu8884` | 架 | ruu8884 |  |
| red-058 | `w0u04` | 瑱 | w0驗 |  |
| red-060 | `wu/w6` | 停 | wu/w6 |  |
| red-061 | `wuw/6` | 停 | wuw/6 |  |
| red-062 | `cjoj6` | 回 | cjoj6 |  |
| red-063 | `cjocjo6` | 回 | cjoc為 |  |
| red-064 | `wu/wu/6` | 停 | wu/wu/6 |  |
| red-065 | `wuwuwu/6` | 停 | wuwu停 |  |
| red-066 | `gkgk6` | gkg額 | gk什 |  |
| red-067 | `hellowuu/6` | hello停 | hellowu營 |  |
| red-068 | `testwu//6` | test停 | testwu//6 |  |
| red-069 | `su;;;;6` | 娘 | su;;;;6 |  |
| red-070 | `wu////6su3` | 停你 | wu////6你 |  |
| red-071 | `wu//su3` | 擰 | wu//su3 |  |
| red-074 | `2u004` | 電 | 2u004 |  |
| red-078 | `wu0004` | 瑱 | wu0004 |  |
| red-080 | `cj8884` | 話 | cj8884 |  |
| red-084 | `ji3a884` | 我罵 | 我a884 |  |
| red-085 | `gkg6` | 什 | gkg6 |  |
| red-086 | `ckck6` | 合 | ck和 |  |
| red-087 | `dkdk4` | 客 | dk可 |  |
| red-088 | `a8a84` | 罵 | a8嗎 |  |
| red-089 | `fufu4` | 器 | fu氣 |  |
| red-090 | `wuwu6` | 提 | wu提 |  |
| red-091 | `cjcj6` | 湖 | cj和 |  |
| red-092 | `gjgj4` | 數 | gj數 |  |
| red-093 | `jojo4` | jojo4 | jo為 |  |
| red-094 | `su3gkgk6` | 你gkg額 | 你gk什 |  |
| red-095 | `gkgkgk6` | gkgkg額 | gkgk什 |  |
| red-096 | `gkkk6` | 什 | gkkk6 |  |
| red-097 | `gggkkk6` | 什 | gggkkk6 |  |
| red-098 | `abcwu//6` | abc停 | abcwu//6 |  |
| red-099 | `qqwu//6` | 停 | qqwu//6 |  |
| red-100 | `thewuuu/6` | the停 | thewuu營 |  |
| red-101 | `qwu//6` | 停 | qwu//6 |  |
| red-102 | `qqwu/6` | 停 | qq停 |  |
| red-103 | `qqqwu/6` | 停 | qqq停 |  |
| red-104 | `ggck6` | gg合 | gg和 |  |
| red-105 | `gcck6` | g合 | gc和 |  |
| red-107 | `sswu/6` | 停 | ss停 |  |
| red-108 | `gkgk4` | gkg惡 | gk設 |  |
| red-109 | `gkgk3` | gkg噁 | gkgk3 |  |
| red-110 | `hellogkgk6` | hellogkg額 | hellogk什 |  |
| red-112 | `ssl␣` | 孬 | ssl␣ |  |
| red-116 | `mmm␣` | ㄩ | mm淤 |  |
| red-119 | `kkk␣` | 痾 | kkk␣ |  |
| red-123 | `ji3ssl␣` | 我孬 | 我ssl␣ |  |
| red-124 | `cjoo␣` | 灰 | cjoo␣ |  |
| red-125 | `wuu/␣` | 聽 | wuu/␣ |  |
| red-126 | `ru88␣` | 家 | ru88␣ |  |
| red-127 | `w00␣` | 攤 | w00␣ |  |
| red-128 | `ji3wu//␣` | 我聽 | 我wu//␣ |  |

</details>

<details><summary>asus-typo：錯 833 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| typo-0001 | `qqwu//6` | 停 | qqwu//6 | redundant-4 |
| typo-0002 | `qwu//6` | 停 | qwu//6 | redundant-4 |
| typo-0003 | `qqwu/6` | 停 | qq停 | redundant-4 |
| typo-0004 | `qqqwu/6` | 停 | qqq停 | redundant-4 |
| typo-0005 | `ggck6` | gg合 | gg和 | redundant-4 |
| typo-0006 | `gcck6` | g合 | gc和 | redundant-4 |
| typo-0008 | `sswu/6` | 停 | ss停 | redundant-4 |
| typo-0009 | `gkgk6` | gkg額 | gk什 | redundant-4 |
| typo-0010 | `gkgk4` | gkg惡 | gk設 | redundant-4 |
| typo-0011 | `gkgk3` | gkg噁 | gkgk3 | redundant-4 |
| typo-0012 | `hellogkgk6` | hellogkg額 | hellogk什 | redundant-4 |
| typo-0013 | `hkg6` | 什 | hkg6 | adj-1 |
| typo-0014 | `fkg6` | 什 | fkg6 | adj-1 |
| typo-0015 | `tkg6` | 什 | tkg6 | adj-1 |
| typo-0016 | `ykg6` | 什 | ykg6 | adj-1 |
| typo-0017 | `bkg6` | 什 | bk時 | adj-1 |
| typo-0018 | `vkg6` | 什 | vkg6 | adj-1 |
| typo-0019 | `ckg6` | c什 | ck時 | adj-1 |
| typo-0020 | `dkg6` | d什 | dkg6 | adj-1 |
| typo-0021 | `xkg6` | x什 | xkg6 | adj-1 |
| typo-0022 | `akg6` | a什 | akg6 | adj-1 |
| typo-0023 | `wkg6` | w什 | wk時 | adj-1 |
| typo-0024 | `ekg6` | e什 | ekg6 | adj-1 |
| typo-0025 | `nkg6` | 什 | nkg6 | adj-1 |
| typo-0026 | `zkg6` | z什 | zkg6 | adj-1 |
| typo-0027 | `hgk6` | 什 | hgk6 | adj-1 |
| typo-0028 | `fgk6` | 什 | fgk6 | adj-1 |
| typo-0029 | `cgk6` | c什 | cg哦 | adj-1 |
| typo-0030 | `dgk6` | d什 | dgk6 | adj-1 |
| typo-0031 | `hkg4` | 設 | hkg4 | adj-1 |
| typo-0032 | `gkh4` | 測 | gkh4 | adj-1 |
| typo-0033 | `gkc4` | g賀 | gkc4 | adj-1 |
| typo-0034 | `dke4` | 各 | dke4 | adj-1 |
| typo-0035 | `s8a4` | 罵 | s8a4 | adj-1 |
| typo-0036 | `a8s4` | 那 | a8s4 | adj-1 |
| typo-0037 | `a8z4` | 法 | a8z4 | adj-1 |
| typo-0038 | `x8a4` | 罵 | x8a4 | adj-1 |
| typo-0039 | `gkl6` | 韶 | gkl6 | adj-1 |
| typo-0040 | `gk84` | 廈 | gk84 | adj-1 |
| typo-0041 | `ji3hkg6` | 我什 | 我hkg6 | adj-1 |
| typo-0042 | `hkg6su3` | 什你 | hkg6你 | adj-1 |
| typo-0043 | `hellohkg6` | helloh什 | hellohkg6 | adj-1 |
| typo-0046 | `hog6` | 誰 | hog6 | adj-1 |
| typo-0047 | `hkg␣` | 奢 | hkg␣ | adj-1 |
| typo-0048 | `hkgk6` | hkg額 | hk什 | adj-1 |
| typo-0049 | `rkg6` | 什 | rkg6 | adj-2 |
| typo-0050 | `xwu/6` | x停 | xw營 | adj-2 |
| typo-0051 | `ewu/6` | 停 | ew營 | adj-2 |
| typo-0052 | `dwu/6` | 停 | dw營 | adj-2 |
| typo-0053 | `cwu/6` | c停 | cw營 | adj-2 |
| typo-0054 | `awu/6` | 停 | aw營 | adj-2 |
| typo-0055 | `zwu/6` | z停 | zw營 | adj-2 |
| typo-0056 | `rwu/6` | r停 | rw營 | adj-2 |
| typo-0057 | `fwu/6` | f停 | fw營 | adj-2 |
| typo-0058 | `vwu/6` | v停 | vw營 | adj-2 |
| typo-0059 | `1wu/6` | 停 | 1停 | adj-2 |
| typo-0060 | `2wu/6` | 停 | 2停 | adj-2 |
| typo-0062 | `gk04` | 善 | gk04 | adj-2 |
| typo-0063 | `gk94` | 曬 | gk94 | adj-2 |
| typo-0064 | `gk;4` | 上 | gk;4 | adj-2 |
| typo-0065 | `gk/4` | 剩 | gk/4 | adj-2 |
| typo-0066 | `gkp4` | 甚 | gkp4 | adj-2 |
| typo-0067 | `su3xwu/6` | 你x停 | 你xw營 | adj-2 |
| typo-0070 | `ewuu/6` | 停 | ewu營 | adj-2 |
| typo-0071 | `cuj4` | 護 | cu誤 | adj-2 |
| typo-0072 | `c8z4` | c法 | c8z4 | adj-2 |
| typo-0077 | `ru4` | 計 | 記 | word |
| typo-0078 | `rm4` | 句 | 據 | word |
| typo-0079 | `suj4` | 怒 | suj4 | word |
| typo-0081 | `sjm3` | 女 | sjm3 | word |
| typo-0082 | `smu3` | 你 | smu3 | word |
| typo-0083 | `rmu4` | 計 | rm一 | word |
| typo-0084 | `rum4` | 句 | rum4 | word |
| typo-0085 | `wuq/6` | 平 | wu朋 | word |
| typo-0086 | `wux/6` | w零 | wux/6 | word |
| typo-0087 | `tygk4` | 設 | ty社 | word |
| typo-0088 | `hhkg6` | 什 | hhkg6 | word |
| typo-0089 | `hkkg6` | 什 | hkkg6 | word |
| typo-0090 | `5tk␣` | 5車 | 5tk␣ | word |
| typo-0092 | `hkg6ak7` | 什麼 | hkg6麼 | word |
| typo-0093 | `ckg6ak7` | c什麼 | ck什麼 | word |
| typo-0094 | `gk6sak7` | 什麼 | 什sak7 | word |
| typo-0095 | `gk6hak7` | 什h麼 | 什ha呃 | word |
| typo-0096 | `ggk6aak7` | 什麼 | ggk6aak7 | word |
| typo-0098 | `g6fru0␣` | 時間 | 時fr殷 | word |
| typo-0099 | `g6cru0␣` | 實c兼 | 時cru0␣ | word |
| typo-0100 | `hg6ru0␣` | 時間 | hg6間 | word |
| typo-0102 | `2u04asl3` | 電as襖 | 電a腦 | word |
| typo-0103 | `2u04gsl3` | 電g腦 | 電gsl3 | word |
| typo-0104 | `2u;04sl3` | 電腦 | 2u;04sl3 | word |
| typo-0105 | `2ju04sl3` | 電腦 | 2ju04sl3 | word |
| typo-0106 | `2mu04sl3` | 電腦 | 2mu04sl3 | word |
| typo-0107 | `2uu004ssl3` | 電腦 | 2uu004ssl3 | word |
| typo-0110 | `ji3hap7` | 我ha嗯 | 我hap7 | word |
| typo-0112 | `vm,6tg/␣` | 學生 | 學tg/␣ | word |
| typo-0113 | `vm,6cg/␣` | 學c生 | 學cg/␣ | word |
| typo-0114 | `cvm,6g/␣` | 學生 | cvm,6聲 | word |
| typo-0115 | `xvm,6g/␣` | x學生 | xvm,6聲 | word |
| typo-0117 | `vu3dcj0␣` | 喜歡 | 喜dc灣 | word |
| typo-0118 | `vu3wcj0␣` | 洗w歡 | 喜wcj0␣ | word |
| typo-0120 | `qwu/6tk␣` | 停車 | qw營車 | word |
| typo-0121 | `xwu/6tk␣` | x停車 | xw營車 | word |
| typo-0122 | `wu/6rtk␣` | 停車 | 停rt阿 | word |
| typo-0123 | `wu/6atk␣` | 停at痾 | 停a車 | word |
| typo-0125 | `hg.3ru␣` | 手機 | hg.3機 | word |
| typo-0126 | `cg.3ru␣` | c手機 | cg偶機 | word |
| typo-0128 | `wu0␣rfu4` | 天氣 | 天rfu4 | word |
| typo-0129 | `wu0␣xfu4` | 天x器 | 天xfu4 | word |
| typo-0131 | `ji3rup␣qwu0␣cp3d9␣vup␣` | 我今天很開心 | 我今qwu0␣很開心 | sentence |
| typo-0132 | `ji3rup␣xwu0␣cp3d9␣vup␣` | 我金x天很開心 | 我今xwu0␣很開心 | sentence |
| typo-0134 | `ji3ruupp␣wu00␣cp3d99␣vup␣` | 我今天很開心 | 我ruupp␣wu00␣很d99␣新 | sentence |
| typo-0135 | `ji3erup␣qwu0␣xcp3sd9␣cvup␣` | 我er陰天很開心 | 我er因qwu0␣xcp3sd9␣cvup␣ | sentence |
| typo-0136 | `ji3arup␣xwu0␣gcp3ad9␣zvup␣` | 我a金x天g很ad唉z新 | 我a今xwu0␣gcp3ad9␣zvup␣ | sentence |
| typo-0138 | `asu3ul41j4ul4u4fu3gfm4rt␣az04` | as以要不要一起去吃飯 | a你要不要憶起gfm4rt␣a反 | sentence |
| typo-0139 | `gsu3ul41j4ul4u4fu3wfm4bt␣hz04` | g你要不要一起w去b吃h飯 | gsu3要不要一起wfm4bt␣hz04 | sentence |
| typo-0141 | `ji3m/4googlebn.␣vmp6` | 我用google搜尋 | 我用googlebn.␣尋 | sentence |
| typo-0143 | `hello␣hkg6ak7` | hello␣什麼 | hello␣hkg6麼 | sentence |
| typo-0144 | `hellohkg6ak7` | helloh什麼 | hellohkg6麼 | sentence |
| typo-0145 | `ji3hkg6ak7` | 我什麼 | 我hkg6麼 | sentence |
| typo-0146 | `cip3` | 很 | cip3 | pair-1 |
| typo-0148 | `2u04zsl3` | 電腦 | 電zsl3 | pair-1 |
| typo-0149 | `ji3qap7` | 我們 | 我qap7 | pair-1 |
| typo-0150 | `ji3vap7` | 我v們 | 我vap7 | pair-1 |
| typo-0151 | `wu/6ytk␣` | 停車 | 停ytk␣ | pair-1 |
| typo-0152 | `wu/6ztk␣` | 停z車 | 停ztk␣ | pair-1 |
| typo-0153 | `zsu3` | 你 | zsu3 | pair-1 |
| typo-0154 | `ji3trup␣` | 我金 | 我tr因 | pair-1 |
| typo-0155 | `erup␣` | er音 | er因 | pair-1 |
| typo-0156 | `trup␣` | 金 | tr因 | pair-1 |
| typo-0157 | `2wu0␣` | 2天 | 2wu0␣ | pair-1 |
| typo-0158 | `1qu0␣` | 1偏 | 1qu0␣ | pair-1 |
| typo-0159 | `2qu0␣` | 2偏 | 2qu0␣ | pair-1 |
| typo-0160 | `5tk4` | 撤 | 5拆 | pair-1 |
| typo-0161 | `5t.4` | 臭 | 5臭 | pair-1 |
| typo-0162 | `1qu/6` | 1平 | 1坪 | pair-1 |
| typo-0163 | `2wu/␣` | 2聽 | 2wu/␣ | pair-1 |
| typo-0164 | `aqu04` | 片 | a片 | pair-1 |
| typo-0165 | `q103` | 版 | q103 | pair-1 |
| typo-0166 | `tvm4` | t續 | tvm4 | pair-1 |
| typo-0167 | `hellobn.␣` | hello搜 | hellobn.␣ | pair-1 |
| typo-0168 | `googlehkg6` | google什 | googlehkg6 | pair-1 |
| typo-0169 | `hellohg.3` | helloh手 | helloh首 | pair-1 |
| typo-0171 | `hellockg6` | helloc什 | hellockg6 | pair-1 |
| typo-0174 | `2wl3` | 討 | 2討 | pair-2 |
| typo-0176 | `5t;6` | 長 | 5場 | pair-2 |
| typo-0177 | `5t;4` | 唱 | 5唱 | pair-2 |
| typo-0179 | `1qj6` | 葡 | 1葡 | pair-2 |
| typo-0180 | `2wk4` | 特 | 2特 | pair-2 |
| typo-0181 | `ohkg6` | oh什 | ohkg6 | pair-2 |
| typo-0182 | `uhkg6` | u什 | uhkg6 | pair-2 |
| typo-0183 | `ihkg6` | 什 | ihkg6 | pair-2 |
| typo-0184 | `testhkg6` | test什 | testhkg6 | pair-2 |
| typo-0185 | `cathkg6` | cat什 | cathkg6 | pair-2 |
| typo-0186 | `hellotkg6` | hello什 | hellotkg6 | pair-2 |
| typo-0187 | `hellohhkg6` | helloh什 | hellohhkg6 | pair-2 |
| typo-0188 | `1l4` | 爆 | 報 | init-1␣ctl␣1 |
| typo-0193 | `w;6` | 堂 | 唐 | init-1␣ctl␣w |
| typo-0194 | `s06` | 難 | 南 | init-1␣ctl␣s |
| typo-0195 | `x06` | 欄 | 藍 | init-1␣ctl␣x |
| typo-0202 | `5/4` | 正 | 政 | init-1␣ctl␣5 |
| typo-0208 | `n04` | 散 | 三 | init-1␣ctl␣n |
| typo-0209 | `1q;6` | 旁 | 1旁 | init-1␣1␣q |
| typo-0212 | `12l4` | 1到 | 12奧 | init-1␣1␣2 |
| typo-0214 | `1s06` | 1難 | 1男 | init-1␣1␣s |
| typo-0215 | `1x06` | 1欄 | 1籃 | init-1␣1␣x |
| typo-0219 | `1ru04` | 1建 | 1件 | init-1␣1␣r |
| typo-0221 | `1vu04` | 1現 | 1線 | init-1␣1␣v |
| typo-0223 | `1t;6` | 1長 | 1場 | init-1␣1␣t |
| typo-0228 | `1n04` | 1散 | 1三 | init-1␣1␣n |
| typo-0229 | `q1l4` | 爆 | q1l4 | init-1␣q␣1 |
| typo-0230 | `qa;6` | 忙 | qa;6 | init-1␣q␣a |
| typo-0231 | `qz;6` | q房 | qz;6 | init-1␣q␣z |
| typo-0232 | `q2l4` | 到 | q2l4 | init-1␣q␣2 |
| typo-0233 | `qw;6` | 堂 | qw;6 | init-1␣q␣w |
| typo-0234 | `qs06` | 難 | qs06 | init-1␣q␣s |
| typo-0235 | `qx06` | q欄 | qx06 | init-1␣q␣x |
| typo-0236 | `qe93` | q改 | qe93 | init-1␣q␣e |
| typo-0237 | `qd04` | q看 | qd04 | init-1␣q␣d |
| typo-0238 | `qc96` | q還 | qc96 | init-1␣q␣c |
| typo-0239 | `qru04` | q建 | qr驗 | init-1␣q␣r |
| typo-0240 | `qfu06` | q前 | qf研 | init-1␣q␣f |
| typo-0241 | `qvu04` | q現 | qvu04 | init-1␣q␣v |
| typo-0243 | `qt;6` | q長 | qt昂 | init-1␣q␣t |
| typo-0244 | `qg;4` | q上 | qg;4 | init-1␣q␣g |
| typo-0245 | `qb;4` | q讓 | qb;4 | init-1␣q␣b |
| typo-0246 | `qy94` | q在 | qy94 | init-1␣q␣y |
| typo-0247 | `qh96` | q才 | qh96 | init-1␣q␣h |
| typo-0248 | `qn04` | q散 | qn04 | init-1␣q␣n |
| typo-0249 | `a1l4` | a爆 | a1澳 | init-1␣a␣1 |
| typo-0250 | `aq;6` | 旁 | a旁 | init-1␣a␣q |
| typo-0251 | `az;6` | 房 | a房 | init-1␣a␣z |
| typo-0252 | `a2l4` | a到 | a2奧 | init-1␣a␣2 |
| typo-0253 | `aw;6` | 堂 | aw昂 | init-1␣a␣w |
| typo-0254 | `as06` | as06 | a男 | init-1␣a␣s |
| typo-0255 | `ax06` | 欄 | a藍 | init-1␣a␣x |
| typo-0257 | `ad04` | ad04 | ad按 | init-1␣a␣d |
| typo-0262 | `a5/4` | a5/4 | a政 | init-1␣a␣5 |
| typo-0263 | `at;6` | at昂 | a長 | init-1␣a␣t |
| typo-0267 | `ah96` | ah96 | a才 | init-1␣a␣h |
| typo-0268 | `an04` | an04 | an暗 | init-1␣a␣n |
| typo-0269 | `z1l4` | z爆 | z1l4 | init-1␣z␣1 |
| typo-0270 | `zq;6` | z旁 | zq;6 | init-1␣z␣q |
| typo-0271 | `za;6` | 忙 | za;6 | init-1␣z␣a |
| typo-0272 | `z2l4` | z到 | z2l4 | init-1␣z␣2 |
| typo-0273 | `zw;6` | z堂 | zw;6 | init-1␣z␣w |
| typo-0274 | `zs06` | 難 | zs06 | init-1␣z␣s |
| typo-0275 | `zx06` | 欄 | zx06 | init-1␣z␣x |
| typo-0276 | `ze93` | z改 | ze93 | init-1␣z␣e |
| typo-0277 | `zd04` | z看 | zd04 | init-1␣z␣d |
| typo-0278 | `zc96` | z還 | zc96 | init-1␣z␣c |
| typo-0279 | `zru04` | z建 | zru04 | init-1␣z␣r |
| typo-0280 | `zfu06` | z前 | zf研 | init-1␣z␣f |
| typo-0281 | `zvu04` | z現 | zvu04 | init-1␣z␣v |
| typo-0283 | `zt;6` | z長 | zt;6 | init-1␣z␣t |
| typo-0284 | `zg;4` | z上 | zg;4 | init-1␣z␣g |
| typo-0285 | `zb;4` | z讓 | zb;4 | init-1␣z␣b |
| typo-0286 | `zy94` | z在 | zy94 | init-1␣z␣y |
| typo-0287 | `zh96` | z才 | zh96 | init-1␣z␣h |
| typo-0288 | `zn04` | z散 | zn04 | init-1␣z␣n |
| typo-0290 | `2q;6` | 旁 | 2旁 | init-1␣2␣q |
| typo-0294 | `2s06` | 2難 | 2男 | init-1␣2␣s |
| typo-0295 | `2x06` | 2欄 | 2籃 | init-1␣2␣x |
| typo-0296 | `2e93` | 改 | 2改 | init-1␣2␣e |
| typo-0299 | `2ru04` | 2建 | 2件 | init-1␣2␣r |
| typo-0301 | `2vu04` | 2現 | 2線 | init-1␣2␣v |
| typo-0303 | `2t;6` | 2長 | 2場 | init-1␣2␣t |
| typo-0308 | `2n04` | 2散 | 2三 | init-1␣2␣n |
| typo-0309 | `w1l4` | 爆 | w1l4 | init-2␣w␣1 |
| typo-0310 | `wq;6` | 旁 | wq;6 | init-2␣w␣q |
| typo-0311 | `wa;6` | 忙 | wa;6 | init-2␣w␣a |
| typo-0312 | `wz;6` | w房 | wz;6 | init-2␣w␣z |
| typo-0313 | `w2l4` | 到 | w2l4 | init-2␣w␣2 |
| typo-0314 | `ws06` | 難 | ws06 | init-2␣w␣s |
| typo-0315 | `wx06` | w欄 | wx06 | init-2␣w␣x |
| typo-0316 | `we93` | we93 | we欸 | init-2␣w␣e |
| typo-0317 | `wd04` | 看 | wd04 | init-2␣w␣d |
| typo-0318 | `wc96` | w還 | wc96 | init-2␣w␣c |
| typo-0319 | `wru/4` | w競 | wr應 | init-2␣w␣r |
| typo-0320 | `wfu06` | w前 | wf鹽 | init-2␣w␣f |
| typo-0321 | `wvu04` | w現 | wvu04 | init-2␣w␣v |
| typo-0322 | `w5k4` | w這 | w5k4 | init-2␣w␣5 |
| typo-0323 | `wt;6` | w長 | wt昂 | init-2␣w␣t |
| typo-0324 | `wg;4` | w上 | wg;4 | init-2␣w␣g |
| typo-0325 | `wb;4` | w讓 | wb;4 | init-2␣w␣b |
| typo-0326 | `wy94` | w在 | wy94 | init-2␣w␣y |
| typo-0327 | `wh96` | w才 | wh96 | init-2␣w␣h |
| typo-0328 | `wn04` | w散 | wn04 | init-2␣w␣n |
| typo-0329 | `s1l4` | s爆 | s1l4 | init-2␣s␣1 |
| typo-0330 | `sq;6` | 旁 | sq昂 | init-2␣s␣q |
| typo-0331 | `sa;6` | 忙 | sa;6 | init-2␣s␣a |
| typo-0332 | `sz;6` | 房 | sz;6 | init-2␣s␣z |
| typo-0333 | `s2l4` | s到 | s2l4 | init-2␣s␣2 |
| typo-0334 | `sw;6` | 堂 | sw;6 | init-2␣s␣w |
| typo-0335 | `sx06` | 欄 | sx06 | init-2␣s␣x |
| typo-0336 | `se93` | 改 | se93 | init-2␣s␣e |
| typo-0337 | `sd04` | 看 | sd04 | init-2␣s␣d |
| typo-0338 | `sc96` | 還 | sc96 | init-2␣s␣c |
| typo-0339 | `sru/4` | s競 | sr應 | init-2␣s␣r |
| typo-0340 | `sfu06` | s前 | sf研 | init-2␣s␣f |
| typo-0341 | `svu04` | s現 | svu04 | init-2␣s␣v |
| typo-0342 | `s5k4` | s這 | s5k4 | init-2␣s␣5 |
| typo-0343 | `st;6` | s長 | st昂 | init-2␣s␣t |
| typo-0344 | `sg;4` | s上 | sg;4 | init-2␣s␣g |
| typo-0345 | `sb;4` | s讓 | sb;4 | init-2␣s␣b |
| typo-0346 | `sy94` | s在 | sy94 | init-2␣s␣y |
| typo-0347 | `sh96` | s才 | sh呆 | init-2␣s␣h |
| typo-0348 | `sn04` | s散 | sn04 | init-2␣s␣n |
| typo-0349 | `x1l4` | x爆 | x1l4 | init-2␣x␣1 |
| typo-0350 | `xq;6` | x旁 | xq;6 | init-2␣x␣q |
| typo-0351 | `xa;6` | 忙 | xa;6 | init-2␣x␣a |
| typo-0352 | `xz;6` | 房 | xz;6 | init-2␣x␣z |
| typo-0353 | `x2l4` | x到 | x2l4 | init-2␣x␣2 |
| typo-0354 | `xw;6` | x堂 | xw;6 | init-2␣x␣w |
| typo-0355 | `xs06` | 難 | xs06 | init-2␣x␣s |
| typo-0356 | `xe93` | x改 | xe93 | init-2␣x␣e |
| typo-0357 | `xd04` | 看 | xd04 | init-2␣x␣d |
| typo-0358 | `xc96` | 還 | xc96 | init-2␣x␣c |
| typo-0359 | `xru/4` | x競 | xr應 | init-2␣x␣r |
| typo-0360 | `xfu06` | x前 | xf研 | init-2␣x␣f |
| typo-0361 | `xvu04` | x現 | xv驗 | init-2␣x␣v |
| typo-0362 | `x5k4` | x這 | x5k4 | init-2␣x␣5 |
| typo-0363 | `xt;6` | x長 | xt;6 | init-2␣x␣t |
| typo-0364 | `xg;4` | x上 | xg;4 | init-2␣x␣g |
| typo-0365 | `xb;4` | x讓 | xb;4 | init-2␣x␣b |
| typo-0366 | `xy94` | x在 | xy94 | init-2␣x␣y |
| typo-0367 | `xh96` | x才 | xh96 | init-2␣x␣h |
| typo-0368 | `xn04` | x散 | xn04 | init-2␣x␣n |
| typo-0369 | `e1l4` | e爆 | e1l4 | init-2␣e␣1 |
| typo-0370 | `eq;6` | e旁 | eq;6 | init-2␣e␣q |
| typo-0371 | `ea;6` | e忙 | ea昂 | init-2␣e␣a |
| typo-0372 | `ez;6` | e房 | ez;6 | init-2␣e␣z |
| typo-0373 | `e2l4` | 到 | e2l4 | init-2␣e␣2 |
| typo-0374 | `ew;6` | 堂 | ew;6 | init-2␣e␣w |
| typo-0375 | `es06` | 難 | es06 | init-2␣e␣s |
| typo-0377 | `ed04` | ed04 | ed暗 | init-2␣e␣d |
| typo-0378 | `ec96` | e還 | ec96 | init-2␣e␣c |
| typo-0379 | `eru/4` | er硬 | er應 | init-2␣e␣r |
| typo-0380 | `efu06` | 前 | ef研 | init-2␣e␣f |
| typo-0381 | `evu04` | e現 | evu04 | init-2␣e␣v |
| typo-0382 | `e5k4` | e這 | e5k4 | init-2␣e␣5 |
| typo-0383 | `et;6` | e長 | et;6 | init-2␣e␣t |
| typo-0384 | `eg;4` | e上 | eg;4 | init-2␣e␣g |
| typo-0385 | `eb;4` | e讓 | eb;4 | init-2␣e␣b |
| typo-0386 | `ey94` | e在 | ey94 | init-2␣e␣y |
| typo-0387 | `eh96` | eh96 | eh癌 | init-2␣e␣h |
| typo-0388 | `en04` | en04 | en暗 | init-2␣e␣n |
| typo-0389 | `d1l4` | d爆 | d1l4 | init-2␣d␣1 |
| typo-0390 | `dq;6` | d旁 | dq;6 | init-2␣d␣q |
| typo-0391 | `da;6` | d忙 | da;6 | init-2␣d␣a |
| typo-0392 | `dz;6` | d房 | dz昂 | init-2␣d␣z |
| typo-0393 | `d2l4` | d到 | d2l4 | init-2␣d␣2 |
| typo-0394 | `dw;6` | 堂 | dw;6 | init-2␣d␣w |
| typo-0395 | `ds06` | 難 | ds06 | init-2␣d␣s |
| typo-0396 | `dx06` | 欄 | dx06 | init-2␣d␣x |
| typo-0398 | `dc96` | 還 | dc呆 | init-2␣d␣c |
| typo-0399 | `dru/4` | 競 | dr應 | init-2␣d␣r |
| typo-0400 | `dfu06` | 前 | df研 | init-2␣d␣f |
| typo-0401 | `dvu04` | 現 | dvu04 | init-2␣d␣v |
| typo-0402 | `d5k4` | d這 | d5k4 | init-2␣d␣5 |
| typo-0403 | `dt;6` | d長 | dt;6 | init-2␣d␣t |
| typo-0404 | `dg;4` | d上 | dg;4 | init-2␣d␣g |
| typo-0405 | `db;4` | d讓 | db盎 | init-2␣d␣b |
| typo-0406 | `dy94` | d在 | dy94 | init-2␣d␣y |
| typo-0407 | `dh96` | d才 | dh96 | init-2␣d␣h |
| typo-0408 | `dn04` | d散 | dn04 | init-2␣d␣n |
| typo-0409 | `c1l4` | c爆 | c1l4 | init-3␣c␣1 |
| typo-0410 | `cq;6` | c旁 | cq;6 | init-3␣c␣q |
| typo-0411 | `ca;6` | c忙 | ca昂 | init-3␣c␣a |
| typo-0412 | `cz;6` | c房 | cz;6 | init-3␣c␣z |
| typo-0413 | `c2l4` | c到 | c2l4 | init-3␣c␣2 |
| typo-0414 | `cw;6` | c堂 | cw昂 | init-3␣c␣w |
| typo-0415 | `cs06` | 難 | cs06 | init-3␣c␣s |
| typo-0416 | `cx06` | 欄 | cx06 | init-3␣c␣x |
| typo-0417 | `ce93` | c改 | ce93 | init-3␣c␣e |
| typo-0418 | `cd04` | 看 | cd04 | init-3␣c␣d |
| typo-0419 | `cru/4` | c競 | cr硬 | init-3␣c␣r |
| typo-0420 | `cfu06` | 前 | cf研 | init-3␣c␣f |
| typo-0421 | `cvu04` | 現 | cvu04 | init-3␣c␣v |
| typo-0422 | `c5k4` | c這 | c5k4 | init-3␣c␣5 |
| typo-0423 | `ct;6` | c長 | ct昂 | init-3␣c␣t |
| typo-0424 | `cg;4` | c上 | cg盎 | init-3␣c␣g |
| typo-0425 | `cb;4` | c讓 | cb;4 | init-3␣c␣b |
| typo-0426 | `cy94` | c在 | cy94 | init-3␣c␣y |
| typo-0427 | `ch96` | c才 | ch挨 | init-3␣c␣h |
| typo-0428 | `cn04` | c散 | cn04 | init-3␣c␣n |
| typo-0429 | `r1l4` | r爆 | r1l4 | init-3␣r␣1 |
| typo-0430 | `rq;6` | r旁 | rq;6 | init-3␣r␣q |
| typo-0431 | `ra;6` | r忙 | ra;6 | init-3␣r␣a |
| typo-0432 | `rz;6` | r房 | rz;6 | init-3␣r␣z |
| typo-0433 | `r2l4` | r到 | r2l4 | init-3␣r␣2 |
| typo-0434 | `rw;6` | r堂 | rw;6 | init-3␣r␣w |
| typo-0435 | `rs06` | r難 | rs06 | init-3␣r␣s |
| typo-0436 | `rx06` | r欄 | rx06 | init-3␣r␣x |
| typo-0437 | `re93` | re93 | re欸 | init-3␣r␣e |
| typo-0438 | `rd04` | 看 | rd案 | init-3␣r␣d |
| typo-0439 | `rc96` | r還 | rc96 | init-3␣r␣c |
| typo-0440 | `rfu06` | 前 | rf研 | init-3␣r␣f |
| typo-0441 | `rvu04` | r現 | rvu04 | init-3␣r␣v |
| typo-0442 | `r5k4` | 這 | r5k4 | init-3␣r␣5 |
| typo-0443 | `rt;6` | 長 | rt昂 | init-3␣r␣t |
| typo-0444 | `rg;4` | 上 | rg;4 | init-3␣r␣g |
| typo-0445 | `rb;4` | r讓 | rb;4 | init-3␣r␣b |
| typo-0446 | `ry94` | r在 | ry94 | init-3␣r␣y |
| typo-0447 | `rh96` | r才 | rh96 | init-3␣r␣h |
| typo-0448 | `rn04` | r散 | rn04 | init-3␣r␣n |
| typo-0449 | `f1l4` | f爆 | f1l4 | init-3␣f␣1 |
| typo-0450 | `fq;6` | f旁 | fq;6 | init-3␣f␣q |
| typo-0451 | `fa;6` | f忙 | fa昂 | init-3␣f␣a |
| typo-0452 | `fz;6` | f房 | fz;6 | init-3␣f␣z |
| typo-0453 | `f2l4` | f到 | f2l4 | init-3␣f␣2 |
| typo-0454 | `fw;6` | f堂 | fw;6 | init-3␣f␣w |
| typo-0455 | `fs06` | f難 | fs06 | init-3␣f␣s |
| typo-0456 | `fx06` | f欄 | fx06 | init-3␣f␣x |
| typo-0457 | `fe93` | 改 | fe93 | init-3␣f␣e |
| typo-0458 | `fd04` | 看 | fd04 | init-3␣f␣d |
| typo-0459 | `fc96` | 還 | fc96 | init-3␣f␣c |
| typo-0460 | `fru/4` | 競 | fr應 | init-3␣f␣r |
| typo-0461 | `fvu04` | 現 | fvu04 | init-3␣f␣v |
| typo-0462 | `f5k4` | f這 | f5k4 | init-3␣f␣5 |
| typo-0463 | `ft;6` | 長 | ft昂 | init-3␣f␣t |
| typo-0464 | `fg;4` | 上 | fg;4 | init-3␣f␣g |
| typo-0465 | `fb;4` | 讓 | fb;4 | init-3␣f␣b |
| typo-0466 | `fy94` | f在 | fy94 | init-3␣f␣y |
| typo-0467 | `fh96` | f才 | fh96 | init-3␣f␣h |
| typo-0468 | `fn04` | f散 | fn04 | init-3␣f␣n |
| typo-0469 | `v1l4` | v爆 | v1l4 | init-3␣v␣1 |
| typo-0470 | `vq;6` | v旁 | vq;6 | init-3␣v␣q |
| typo-0471 | `va;6` | v忙 | va;6 | init-3␣v␣a |
| typo-0472 | `vz;6` | v房 | vz;6 | init-3␣v␣z |
| typo-0473 | `v2l4` | v到 | v2l4 | init-3␣v␣2 |
| typo-0474 | `vw;6` | v堂 | vw;6 | init-3␣v␣w |
| typo-0475 | `vs06` | v難 | vs06 | init-3␣v␣s |
| typo-0476 | `vx06` | v欄 | vx06 | init-3␣v␣x |
| typo-0477 | `ve93` | v改 | ve93 | init-3␣v␣e |
| typo-0478 | `vd04` | 看 | vd04 | init-3␣v␣d |
| typo-0479 | `vc96` | 還 | vc96 | init-3␣v␣c |
| typo-0480 | `vru/4` | v競 | vr應 | init-3␣v␣r |
| typo-0481 | `vfu06` | 前 | vf研 | init-3␣v␣f |
| typo-0482 | `v5k4` | v這 | v5k4 | init-3␣v␣5 |
| typo-0483 | `vt;6` | v長 | vt;6 | init-3␣v␣t |
| typo-0484 | `vg;4` | 上 | vg;4 | init-3␣v␣g |
| typo-0485 | `vb;4` | 讓 | vb盎 | init-3␣v␣b |
| typo-0486 | `vy94` | v在 | vy94 | init-3␣v␣y |
| typo-0487 | `vh96` | v才 | vh96 | init-3␣v␣h |
| typo-0488 | `vn04` | v散 | vn04 | init-3␣v␣n |
| typo-0489 | `51l4` | 5爆 | 51澳 | init-3␣5␣1 |
| typo-0493 | `52l4` | 5到 | 52澳 | init-3␣5␣2 |
| typo-0495 | `5s06` | 5難 | 5男 | init-3␣5␣s |
| typo-0496 | `5x06` | 5欄 | 5籃 | init-3␣5␣x |
| typo-0500 | `5ru/4` | 競 | 5淨 | init-3␣5␣r |
| typo-0503 | `5t;6` | 長 | 5場 | init-3␣5␣t |
| typo-0506 | `5y94` | 在 | 5在 | init-3␣5␣y |
| typo-0508 | `5n04` | 5散 | 5三 | init-3␣5␣n |
| typo-0509 | `t1l4` | t爆 | t1l4 | init-4␣t␣1 |
| typo-0510 | `tq;6` | t旁 | tq;6 | init-4␣t␣q |
| typo-0512 | `tz;6` | t房 | tz;6 | init-4␣t␣z |
| typo-0513 | `t2l4` | t到 | t2l4 | init-4␣t␣2 |
| typo-0514 | `tw;6` | t堂 | tw;6 | init-4␣t␣w |
| typo-0515 | `ts06` | t難 | ts06 | init-4␣t␣s |
| typo-0516 | `tx06` | t欄 | tx06 | init-4␣t␣x |
| typo-0517 | `te93` | t改 | te93 | init-4␣t␣e |
| typo-0518 | `td04` | t看 | td04 | init-4␣t␣d |
| typo-0519 | `tc96` | t還 | tc96 | init-4␣t␣c |
| typo-0520 | `tru/4` | 競 | tr應 | init-4␣t␣r |
| typo-0521 | `tfu06` | 前 | tf研 | init-4␣t␣f |
| typo-0522 | `tvu04` | t現 | tvu04 | init-4␣t␣v |
| typo-0523 | `t5k4` | 這 | t5k4 | init-4␣t␣5 |
| typo-0524 | `tg;4` | 上 | tg;4 | init-4␣t␣g |
| typo-0525 | `tb;4` | t讓 | tb盎 | init-4␣t␣b |
| typo-0526 | `ty94` | 在 | ty94 | init-4␣t␣y |
| typo-0527 | `th96` | 才 | th96 | init-4␣t␣h |
| typo-0528 | `tn04` | t散 | tn案 | init-4␣t␣n |
| typo-0529 | `g1l4` | g爆 | g1l4 | init-4␣g␣1 |
| typo-0530 | `gq;6` | g旁 | gq;6 | init-4␣g␣q |
| typo-0531 | `ga;6` | g忙 | ga;6 | init-4␣g␣a |
| typo-0532 | `gz;6` | g房 | gz;6 | init-4␣g␣z |
| typo-0533 | `g2l4` | g到 | g2l4 | init-4␣g␣2 |
| typo-0534 | `gw;6` | g堂 | gw;6 | init-4␣g␣w |
| typo-0535 | `gs06` | g難 | gs06 | init-4␣g␣s |
| typo-0536 | `gx06` | g欄 | gx06 | init-4␣g␣x |
| typo-0537 | `ge93` | g改 | ge93 | init-4␣g␣e |
| typo-0538 | `gd04` | g看 | gd04 | init-4␣g␣d |
| typo-0539 | `gc96` | g還 | gc96 | init-4␣g␣c |
| typo-0540 | `gru/4` | 競 | gr硬 | init-4␣g␣r |
| typo-0541 | `gfu06` | 前 | gf研 | init-4␣g␣f |
| typo-0542 | `gvu04` | 現 | gvu04 | init-4␣g␣v |
| typo-0543 | `g5k4` | g這 | g5k4 | init-4␣g␣5 |
| typo-0544 | `gt;6` | 長 | gt昂 | init-4␣g␣t |
| typo-0545 | `gb;4` | 讓 | gb;4 | init-4␣g␣b |
| typo-0546 | `gy94` | 在 | gy94 | init-4␣g␣y |
| typo-0547 | `gh96` | 才 | gh96 | init-4␣g␣h |
| typo-0548 | `gn04` | 散 | gn04 | init-4␣g␣n |
| typo-0549 | `b1l4` | b爆 | b1l4 | init-4␣b␣1 |
| typo-0550 | `bq;6` | b旁 | bq;6 | init-4␣b␣q |
| typo-0551 | `ba;6` | b忙 | ba;6 | init-4␣b␣a |
| typo-0552 | `bz;6` | b房 | bz;6 | init-4␣b␣z |
| typo-0553 | `b2l4` | b到 | b2l4 | init-4␣b␣2 |
| typo-0554 | `bw;6` | b堂 | bw;6 | init-4␣b␣w |
| typo-0555 | `bs06` | b難 | bs06 | init-4␣b␣s |
| typo-0556 | `bx06` | b欄 | bx06 | init-4␣b␣x |
| typo-0557 | `be93` | be93 | be欸 | init-4␣b␣e |
| typo-0558 | `bd04` | b看 | bd04 | init-4␣b␣d |
| typo-0559 | `bc96` | b還 | bc96 | init-4␣b␣c |
| typo-0560 | `bru/4` | b競 | br應 | init-4␣b␣r |
| typo-0561 | `bfu06` | 前 | bf研 | init-4␣b␣f |
| typo-0562 | `bvu04` | 現 | bvu04 | init-4␣b␣v |
| typo-0563 | `b5k4` | b這 | b5k4 | init-4␣b␣5 |
| typo-0564 | `bt;6` | b長 | bt;6 | init-4␣b␣t |
| typo-0565 | `bg;4` | 上 | bg;4 | init-4␣b␣g |
| typo-0566 | `by94` | by94 | by愛 | init-4␣b␣y |
| typo-0567 | `bh96` | 才 | bh96 | init-4␣b␣h |
| typo-0568 | `bn04` | 散 | bn04 | init-4␣b␣n |
| typo-0569 | `y1l4` | y爆 | y1l4 | init-4␣y␣1 |
| typo-0570 | `yq;6` | y旁 | yq;6 | init-4␣y␣q |
| typo-0571 | `ya;6` | y忙 | ya昂 | init-4␣y␣a |
| typo-0572 | `yz;6` | y房 | yz;6 | init-4␣y␣z |
| typo-0573 | `y2l4` | y到 | y2l4 | init-4␣y␣2 |
| typo-0574 | `yw;6` | y堂 | yw;6 | init-4␣y␣w |
| typo-0575 | `ys06` | y難 | ys06 | init-4␣y␣s |
| typo-0576 | `yx06` | y欄 | yx06 | init-4␣y␣x |
| typo-0577 | `ye93` | ye93 | ye欸 | init-4␣y␣e |
| typo-0578 | `yd04` | y看 | yd按 | init-4␣y␣d |
| typo-0579 | `yc96` | y還 | yc96 | init-4␣y␣c |
| typo-0580 | `yru/4` | y競 | yr應 | init-4␣y␣r |
| typo-0581 | `yfu06` | y前 | yf研 | init-4␣y␣f |
| typo-0582 | `yvu04` | y現 | yvu04 | init-4␣y␣v |
| typo-0583 | `y5k4` | 這 | y5k4 | init-4␣y␣5 |
| typo-0584 | `yt;6` | 長 | yt;6 | init-4␣y␣t |
| typo-0585 | `yg;4` | 上 | yg;4 | init-4␣y␣g |
| typo-0586 | `yb;4` | y讓 | yb;4 | init-4␣y␣b |
| typo-0587 | `yh96` | 才 | yh96 | init-4␣y␣h |
| typo-0588 | `yn04` | y散 | yn04 | init-4␣y␣n |
| typo-0589 | `h1l4` | h爆 | h1l4 | init-4␣h␣1 |
| typo-0590 | `hq;6` | h旁 | hq;6 | init-4␣h␣q |
| typo-0592 | `hz;6` | h房 | hz;6 | init-4␣h␣z |
| typo-0593 | `h2l4` | h到 | h2l4 | init-4␣h␣2 |
| typo-0594 | `hw;6` | h堂 | hw;6 | init-4␣h␣w |
| typo-0595 | `hs06` | h難 | hs06 | init-4␣h␣s |
| typo-0596 | `hx06` | h欄 | hx06 | init-4␣h␣x |
| typo-0597 | `he93` | he93 | he欸 | init-4␣h␣e |
| typo-0598 | `hd04` | h看 | hd04 | init-4␣h␣d |
| typo-0599 | `hc96` | h還 | hc96 | init-4␣h␣c |
| typo-0600 | `hru/4` | hr硬 | hr應 | init-4␣h␣r |
| typo-0601 | `hfu06` | h前 | hf研 | init-4␣h␣f |
| typo-0602 | `hvu04` | h現 | hvu04 | init-4␣h␣v |
| typo-0603 | `h5k4` | h這 | h5k4 | init-4␣h␣5 |
| typo-0604 | `ht;6` | 長 | ht昂 | init-4␣h␣t |
| typo-0605 | `hg;4` | 上 | hg;4 | init-4␣h␣g |
| typo-0606 | `hb;4` | 讓 | hb;4 | init-4␣h␣b |
| typo-0607 | `hy94` | 在 | hy94 | init-4␣h␣y |
| typo-0608 | `hn04` | 散 | hn04 | init-4␣h␣n |
| typo-0609 | `n1l4` | n爆 | n1l4 | init-4␣n␣1 |
| typo-0610 | `nq;6` | n旁 | nq;6 | init-4␣n␣q |
| typo-0611 | `na;6` | n忙 | na;6 | init-4␣n␣a |
| typo-0612 | `nz;6` | n房 | nz;6 | init-4␣n␣z |
| typo-0613 | `n2l4` | n到 | n2l4 | init-4␣n␣2 |
| typo-0614 | `nw;6` | n堂 | nw;6 | init-4␣n␣w |
| typo-0615 | `ns06` | n難 | ns06 | init-4␣n␣s |
| typo-0616 | `nx06` | n欄 | nx06 | init-4␣n␣x |
| typo-0617 | `ne93` | n改 | ne93 | init-4␣n␣e |
| typo-0618 | `nd04` | n看 | nd04 | init-4␣n␣d |
| typo-0619 | `nc96` | n還 | nc96 | init-4␣n␣c |
| typo-0620 | `nru/4` | n競 | nr應 | init-4␣n␣r |
| typo-0621 | `nfu06` | n前 | nf延 | init-4␣n␣f |
| typo-0622 | `nvu04` | n現 | nvu04 | init-4␣n␣v |
| typo-0623 | `n5k4` | n這 | n5k4 | init-4␣n␣5 |
| typo-0624 | `nt;6` | nt昂 | nt;6 | init-4␣n␣t |
| typo-0625 | `ng;4` | 上 | ng;4 | init-4␣n␣g |
| typo-0626 | `nb;4` | 讓 | nb;4 | init-4␣n␣b |
| typo-0627 | `ny94` | n在 | ny94 | init-4␣n␣y |
| typo-0628 | `nh96` | 才 | nh96 | init-4␣n␣h |
| typo-0629 | `a0s6` | 難 | a0s6 | fix-medial-order␣afbt␣a␣s |
| typo-0630 | `a0d4` | a看 | a0d4 | fix-medial-order␣afbt␣a␣d |
| typo-0631 | `a;t6` | a長 | a;持 | fix-medial-order␣afbt␣a␣t |
| typo-0632 | `a9h6` | a才 | a9詞 | fix-medial-order␣afbt␣a␣h |
| typo-0633 | `a0n4` | a散 | a0四 | fix-medial-order␣afbt␣a␣n |
| typo-0634 | `w9e3` | 改 | w9e3 | fix-medial-order␣afbt␣w␣e |
| typo-0635 | `e0x6` | e欄 | e0x6 | fix-medial-order␣afbt␣e␣x |
| typo-0636 | `e0d4` | 看 | e0d4 | fix-medial-order␣afbt␣e␣d |
| typo-0637 | `eu/r4` | 競 | eu/r4 | fix-medial-order␣afbt␣e␣r |
| typo-0638 | `e9h6` | e才 | e9辭 | fix-medial-order␣afbt␣e␣h |
| typo-0639 | `e0n4` | e散 | e0四 | fix-medial-order␣afbt␣e␣n |
| typo-0640 | `d9e3` | 改 | d9e3 | fix-medial-order␣afbt␣d␣e |
| typo-0641 | `r9e3` | 改 | r9e3 | fix-medial-order␣afbt␣r␣e |
| typo-0642 | `t;a6` | t忙 | t;a6 | fix-medial-order␣afbt␣t␣a |
| typo-0643 | `b9e3` | b改 | b9e3 | fix-medial-order␣afbt␣b␣e |
| typo-0644 | `b9y4` | b在 | b9自 | fix-medial-order␣afbt␣b␣y |
| typo-0645 | `y9e3` | y改 | y9e3 | fix-medial-order␣afbt␣y␣e |
| typo-0646 | `h;a6` | h忙 | h;a6 | fix-medial-order␣afbt␣h␣a |
| typo-0647 | `h9e3` | h改 | h9e3 | fix-medial-order␣afbt␣h␣e |
| typo-0648 | `hu/r4` | h競 | hu/r4 | fix-medial-order␣afbt␣h␣r |
| typo-0649 | `n;t6` | n長 | n;t6 | fix-medial-order␣afbt␣n␣t |
| typo-0651 | `q5k4` | q這 | q5k4 | fix-medial-order␣init␣q␣5 |
| typo-0653 | `z5k4` | z這 | z5k4 | fix-medial-order␣init␣z␣5 |
| typo-0656 | `1w;3` | 躺 | 1躺 | fix-medial-order␣digit␣1␣w |
| typo-0657 | `2w;3` | 躺 | 2躺 | fix-medial-order␣digit␣2␣w |
| typo-0658 | `12l3` | 12襖 | 1島 | fix-medial-order␣digit␣1␣2 |
| typo-0661 | `sju3` | 你 | sju3 | fix-medial-order␣medial␣j␣u |
| typo-0662 | `xju4` | 力 | xju4 | fix-medial-order␣medial␣j␣u |
| typo-0663 | `smu3` | 你 | smu3 | fix-medial-order␣medial␣m␣u |
| typo-0664 | `xmu4` | 力 | xmu4 | fix-medial-order␣medial␣m␣u |
| typo-0665 | `suj4` | 怒 | suj4 | fix-medial-order␣medial␣u␣j |
| typo-0666 | `cuj4` | 護 | cu誤 | fix-medial-order␣medial␣u␣j |
| typo-0667 | `smj4` | 怒 | smj4 | fix-medial-order␣medial␣m␣j |
| typo-0670 | `xum4` | 率 | xum4 | fix-medial-order␣medial␣u␣m |
| typo-0671 | `sjm3` | 女 | sjm3 | fix-medial-order␣medial␣j␣m |
| typo-0672 | `xjm4` | 率 | xjm4 | fix-medial-order␣medial␣j␣m |
| typo-0674 | `xu4` | 力 | 立 | fix-medial-order␣ctl␣xu4 |
| typo-0676 | `cj4` | 護 | 戶 | fix-medial-order␣ctl␣cj4 |
| typo-0678 | `xm4` | 率 | 綠 | fix-medial-order␣ctl␣xm4 |
| typo-0680 | `0w4` | 探 | 0w4 | fix-medial-order␣order␣0w4 |
| typo-0681 | `8a4` | 罵 | 8a4 | fix-medial-order␣order␣8a4 |
| typo-0683 | `.g3` | 手 | 首 | fix-medial-order␣order␣.g3 |
| typo-0685 | `/g4` | 剩 | 勝 | fix-medial-order␣order␣/g4 |
| typo-0690 | `jc4` | 護 | 戶 | fix-medial-order␣order␣jc4 |
| typo-0691 | `mx4` | 率 | 綠 | fix-medial-order␣order␣mx4 |
| typo-0692 | `ji3kg6` | 我什 | 我蛇 | fix-medial-order␣order␣ji3kg6 |
| typo-0695 | `ur0␣` | 兼 | 間 | fix-medial-order␣order␣ur0␣ |
| typo-0696 | `ju3` | 以 | ju3 | fix-medial-order␣order␣ju3 |
| typo-0699 | `gk4` | 設 | 社 | final␣ctl␣k |
| typo-0704 | `g.3` | 手 | 首 | final␣ctl␣. |
| typo-0706 | `gp6` | 神 | 什 | final␣ctl␣p |
| typo-0708 | `g/4` | 剩 | 勝 | final␣ctl␣/ |
| typo-0710 | `gi83` | 傻 | gi83 | final␣final␣i␣8 |
| typo-0711 | `gk83` | 傻 | gk83 | final␣final␣k␣8 |
| typo-0712 | `g,83` | 傻 | g,83 | final␣final␣,␣8 |
| typo-0713 | `g983` | 傻 | g983 | final␣final␣9␣8 |
| typo-0715 | `gl83` | 傻 | gl83 | final␣final␣l␣8 |
| typo-0716 | `g.83` | 傻 | g.83 | final␣final␣.␣8 |
| typo-0717 | `g083` | 傻 | g083 | final␣final␣0␣8 |
| typo-0718 | `gp83` | 傻 | gp83 | final␣final␣p␣8 |
| typo-0719 | `g;83` | 傻 | g;83 | final␣final␣;␣8 |
| typo-0720 | `g/83` | 傻 | g/83 | final␣final␣/␣8 |
| typo-0721 | `g-83` | 傻 | g-83 | final␣final␣-␣8 |
| typo-0722 | `q8i4` | 破 | q8i4 | final␣final␣8␣i |
| typo-0723 | `qki4` | 破 | qki4 | final␣final␣k␣i |
| typo-0724 | `q,i4` | 破 | q,i4 | final␣final␣,␣i |
| typo-0725 | `q9i4` | 破 | q9i4 | final␣final␣9␣i |
| typo-0726 | `qoi4` | 破 | qoi4 | final␣final␣o␣i |
| typo-0727 | `qli4` | 破 | qli4 | final␣final␣l␣i |
| typo-0728 | `q.i4` | 破 | q.i4 | final␣final␣.␣i |
| typo-0729 | `q0i4` | 破 | q0i4 | final␣final␣0␣i |
| typo-0730 | `qpi4` | 破 | qpi4 | final␣final␣p␣i |
| typo-0731 | `q;i4` | 破 | q;i4 | final␣final␣;␣i |
| typo-0732 | `q/i4` | 破 | q/i4 | final␣final␣/␣i |
| typo-0733 | `q-i4` | 破 | q-i4 | final␣final␣-␣i |
| typo-0734 | `g8k4` | 設 | g8俄 | final␣final␣8␣k |
| typo-0735 | `gik4` | 設 | gik4 | final␣final␣i␣k |
| typo-0736 | `g,k4` | 設 | g,k4 | final␣final␣,␣k |
| typo-0737 | `g9k4` | 設 | g9俄 | final␣final␣9␣k |
| typo-0738 | `gok4` | go惡 | go俄 | final␣final␣o␣k |
| typo-0739 | `glk4` | 設 | glk4 | final␣final␣l␣k |
| typo-0740 | `g.k4` | 設 | g.k4 | final␣final␣.␣k |
| typo-0741 | `g0k4` | 設 | g0俄 | final␣final␣0␣k |
| typo-0742 | `gpk4` | 設 | gpk4 | final␣final␣p␣k |
| typo-0743 | `g;k4` | 設 | g;k4 | final␣final␣;␣k |
| typo-0744 | `g/k4` | 設 | g/k4 | final␣final␣/␣k |
| typo-0745 | `g-k4` | 設 | g-k4 | final␣final␣-␣k |
| typo-0746 | `vu8,4` | 謝 | vu8,4 | final␣final␣8␣, |
| typo-0747 | `vui,4` | 謝 | vui,4 | final␣final␣i␣, |
| typo-0748 | `vuk,4` | 謝 | vuk,4 | final␣final␣k␣, |
| typo-0749 | `vu9,4` | 謝 | vu9,4 | final␣final␣9␣, |
| typo-0750 | `vuo,4` | 謝 | vuo,4 | final␣final␣o␣, |
| typo-0751 | `vul,4` | 謝 | vul,4 | final␣final␣l␣, |
| typo-0752 | `vu.,4` | 謝 | vu.,4 | final␣final␣.␣, |
| typo-0753 | `vu0,4` | 謝 | vu0,4 | final␣final␣0␣, |
| typo-0754 | `vup,4` | 謝 | vup,4 | final␣final␣p␣, |
| typo-0755 | `vu;,4` | 謝 | vu;,4 | final␣final␣;␣, |
| typo-0756 | `vu/,4` | 謝 | vu/,4 | final␣final␣/␣, |
| typo-0757 | `vu-,4` | 謝 | vu-,4 | final␣final␣-␣, |
| typo-0758 | `g894` | 曬 | g894 | final␣final␣8␣9 |
| typo-0759 | `gi94` | 曬 | gi94 | final␣final␣i␣9 |
| typo-0760 | `gk94` | 曬 | gk94 | final␣final␣k␣9 |
| typo-0761 | `g,94` | 曬 | g,94 | final␣final␣,␣9 |
| typo-0762 | `go94` | go94 | go愛 | final␣final␣o␣9 |
| typo-0763 | `gl94` | 曬 | gl94 | final␣final␣l␣9 |
| typo-0764 | `g.94` | 曬 | g.94 | final␣final␣.␣9 |
| typo-0765 | `g094` | 曬 | g094 | final␣final␣0␣9 |
| typo-0766 | `gp94` | 曬 | gp94 | final␣final␣p␣9 |
| typo-0767 | `g;94` | 曬 | g;94 | final␣final␣;␣9 |
| typo-0768 | `g/94` | 曬 | g/94 | final␣final␣/␣9 |
| typo-0769 | `g-94` | 曬 | g-94 | final␣final␣-␣9 |
| typo-0770 | `g8o6` | 誰 | g8o6 | final␣final␣8␣o |
| typo-0771 | `gio6` | 誰 | gio6 | final␣final␣i␣o |
| typo-0772 | `gko6` | 誰 | gko6 | final␣final␣k␣o |
| typo-0773 | `g,o6` | 誰 | g,o6 | final␣final␣,␣o |
| typo-0774 | `g9o6` | 誰 | g9o6 | final␣final␣9␣o |
| typo-0775 | `glo6` | 誰 | glo6 | final␣final␣l␣o |
| typo-0776 | `g.o6` | 誰 | g.o6 | final␣final␣.␣o |
| typo-0777 | `g0o6` | 誰 | g0o6 | final␣final␣0␣o |
| typo-0778 | `gpo6` | 誰 | gpo6 | final␣final␣p␣o |
| typo-0779 | `g;o6` | 誰 | g;o6 | final␣final␣;␣o |
| typo-0780 | `g/o6` | 誰 | g/o6 | final␣final␣/␣o |
| typo-0781 | `g-o6` | 誰 | g-o6 | final␣final␣-␣o |
| typo-0782 | `g8l3` | 少 | g8l3 | final␣final␣8␣l |
| typo-0783 | `gil3` | 少 | gil3 | final␣final␣i␣l |
| typo-0784 | `gkl3` | 少 | gkl3 | final␣final␣k␣l |
| typo-0785 | `g,l3` | 少 | g,l3 | final␣final␣,␣l |
| typo-0786 | `g9l3` | 少 | g9l3 | final␣final␣9␣l |
| typo-0787 | `gol3` | go襖 | go夭 | final␣final␣o␣l |
| typo-0788 | `g.l3` | 少 | g.l3 | final␣final␣.␣l |
| typo-0789 | `g0l3` | 少 | g0l3 | final␣final␣0␣l |
| typo-0790 | `gpl3` | 少 | gpl3 | final␣final␣p␣l |
| typo-0791 | `g;l3` | 少 | g;l3 | final␣final␣;␣l |
| typo-0792 | `g/l3` | 少 | g/l3 | final␣final␣/␣l |
| typo-0793 | `g-l3` | 少 | g-l3 | final␣final␣-␣l |
| typo-0794 | `g8.3` | 手 | g8.3 | final␣final␣8␣. |
| typo-0795 | `gi.3` | 手 | gi.3 | final␣final␣i␣. |
| typo-0796 | `gk.3` | 手 | gk.3 | final␣final␣k␣. |
| typo-0797 | `g,.3` | 手 | g,.3 | final␣final␣,␣. |
| typo-0798 | `g9.3` | 手 | g9.3 | final␣final␣9␣. |
| typo-0799 | `go.3` | go嘔 | go歐 | final␣final␣o␣. |
| typo-0800 | `gl.3` | 手 | gl.3 | final␣final␣l␣. |
| typo-0801 | `g0.3` | 手 | g0.3 | final␣final␣0␣. |
| typo-0802 | `gp.3` | 手 | gp.3 | final␣final␣p␣. |
| typo-0803 | `g;.3` | 手 | g;.3 | final␣final␣;␣. |
| typo-0804 | `g/.3` | 手 | g/.3 | final␣final␣/␣. |
| typo-0805 | `g-.3` | 手 | g-.3 | final␣final␣-␣. |
| typo-0806 | `g804` | 善 | g804 | final␣final␣8␣0 |
| typo-0807 | `gi04` | 善 | gi04 | final␣final␣i␣0 |
| typo-0808 | `gk04` | 善 | gk04 | final␣final␣k␣0 |
| typo-0809 | `g,04` | 善 | g,04 | final␣final␣,␣0 |
| typo-0810 | `g904` | 善 | g904 | final␣final␣9␣0 |
| typo-0811 | `go04` | go04 | go案 | final␣final␣o␣0 |
| typo-0812 | `gl04` | 善 | gl04 | final␣final␣l␣0 |
| typo-0813 | `g.04` | 善 | g.04 | final␣final␣.␣0 |
| typo-0814 | `gp04` | 善 | gp04 | final␣final␣p␣0 |
| typo-0815 | `g;04` | 善 | g;04 | final␣final␣;␣0 |
| typo-0816 | `g/04` | 善 | g/04 | final␣final␣/␣0 |
| typo-0817 | `g-04` | 善 | g-04 | final␣final␣-␣0 |
| typo-0818 | `g8p6` | 神 | g8p6 | final␣final␣8␣p |
| typo-0819 | `gip6` | 神 | gip6 | final␣final␣i␣p |
| typo-0820 | `gkp6` | 神 | gkp6 | final␣final␣k␣p |
| typo-0821 | `g,p6` | 神 | g,p6 | final␣final␣,␣p |
| typo-0822 | `g9p6` | 神 | g9p6 | final␣final␣9␣p |
| typo-0823 | `gop6` | 神 | gop6 | final␣final␣o␣p |
| typo-0824 | `glp6` | 神 | glp6 | final␣final␣l␣p |
| typo-0825 | `g.p6` | 神 | g.p6 | final␣final␣.␣p |
| typo-0826 | `g0p6` | 神 | g0p6 | final␣final␣0␣p |
| typo-0827 | `g;p6` | 神 | g;p6 | final␣final␣;␣p |
| typo-0828 | `g/p6` | 神 | g/p6 | final␣final␣/␣p |
| typo-0829 | `g-p6` | 神 | g-p6 | final␣final␣-␣p |
| typo-0830 | `g8;4` | 上 | g8;4 | final␣final␣8␣; |
| typo-0831 | `gi;4` | 上 | gi;4 | final␣final␣i␣; |
| typo-0832 | `gk;4` | 上 | gk;4 | final␣final␣k␣; |
| typo-0833 | `g,;4` | 上 | g,;4 | final␣final␣,␣; |
| typo-0834 | `g9;4` | 上 | g9;4 | final␣final␣9␣; |
| typo-0836 | `gl;4` | 上 | gl;4 | final␣final␣l␣; |
| typo-0837 | `g.;4` | 上 | g.;4 | final␣final␣.␣; |
| typo-0838 | `g0;4` | 上 | g0;4 | final␣final␣0␣; |
| typo-0839 | `gp;4` | 上 | gp;4 | final␣final␣p␣; |
| typo-0840 | `g/;4` | 上 | g/;4 | final␣final␣/␣; |
| typo-0841 | `g-;4` | 上 | g-;4 | final␣final␣-␣; |
| typo-0842 | `g8/4` | 剩 | g8/4 | final␣final␣8␣/ |
| typo-0843 | `gi/4` | 剩 | gi/4 | final␣final␣i␣/ |
| typo-0844 | `gk/4` | 剩 | gk/4 | final␣final␣k␣/ |
| typo-0845 | `g,/4` | 剩 | g,/4 | final␣final␣,␣/ |
| typo-0846 | `g9/4` | 剩 | g9/4 | final␣final␣9␣/ |
| typo-0847 | `go/4` | 剩 | go/4 | final␣final␣o␣/ |
| typo-0848 | `gl/4` | 剩 | gl/4 | final␣final␣l␣/ |
| typo-0849 | `g./4` | 剩 | g./4 | final␣final␣.␣/ |
| typo-0850 | `g0/4` | 剩 | g0/4 | final␣final␣0␣/ |
| typo-0851 | `gp/4` | 剩 | gp/4 | final␣final␣p␣/ |
| typo-0852 | `g;/4` | 剩 | g;/4 | final␣final␣;␣/ |
| typo-0853 | `g-/4` | 剩 | g-/4 | final␣final␣-␣/ |
| typo-0855 | `i-4` | i-4 | i二 | final␣final␣i␣- |
| typo-0857 | `,-4` | ,二 | ,-4 | final␣final␣,␣- |
| typo-0861 | `.-4` | .二 | .-4 | final␣final␣.␣- |
| typo-0863 | `p-4` | 二 | p-4 | final␣final␣p␣- |
| typo-0864 | `;-4` | ;二 | ;-4 | final␣final␣;␣- |
| typo-0865 | `/-4` | /二 | /-4 | final␣final␣/␣- |
| typo-0866 | `xo84` | 辣 | xo84 | final-repeat-combo␣final␣o␣8␣x |
| typo-0867 | `xok4` | 樂 | xok4 | final-repeat-combo␣final␣o␣k␣x |
| typo-0868 | `xo96` | 來 | xo96 | final-repeat-combo␣final␣o␣9␣x |
| typo-0869 | `xol3` | 老 | xol3 | final-repeat-combo␣final␣o␣l␣x |
| typo-0870 | `xo.4` | 漏 | xo.4 | final-repeat-combo␣final␣o␣.␣x |
| typo-0871 | `xo04` | 爛 | xo04 | final-repeat-combo␣final␣o␣0␣x |
| typo-0872 | `xo;4` | 浪 | xo;4 | final-repeat-combo␣final␣o␣;␣x |
| typo-0873 | `a83` | 碼 | 馬 | final-repeat-combo␣rep␣8␣plain␣x1␣tone |
| typo-0874 | `a883` | 碼 | a883 | final-repeat-combo␣rep␣8␣plain␣x2␣tone |
| typo-0878 | `a88␣` | 嗎 | a88␣ | final-repeat-combo␣rep␣8␣plain␣x2␣space |
| typo-0882 | `ru84` | 架 | 價 | final-repeat-combo␣rep␣8␣medial␣x1␣tone |
| typo-0883 | `ru884` | 架 | ru884 | final-repeat-combo␣rep␣8␣medial␣x2␣tone |
| typo-0884 | `ru8884` | 架 | ru8884 | final-repeat-combo␣rep␣8␣medial␣x3␣tone |
| typo-0887 | `ru88␣` | 家 | ru88␣ | final-repeat-combo␣rep␣8␣medial␣x2␣space |
| typo-0888 | `ru888␣` | 家 | ru888␣ | final-repeat-combo␣rep␣8␣medial␣x3␣space |
| typo-0892 | `s994` | 奈 | s994 | final-repeat-combo␣rep␣9␣plain␣x2␣tone |
| typo-0901 | `cj994` | 壞 | cj994 | final-repeat-combo␣rep␣9␣medial␣x2␣tone |
| typo-0902 | `cj9994` | 壞 | cj9994 | final-repeat-combo␣rep␣9␣medial␣x3␣tone |
| typo-0905 | `cj99␣` | c歪 | cj99␣ | final-repeat-combo␣rep␣9␣medial␣x2␣space |
| typo-0910 | `w004` | 探 | w004 | final-repeat-combo␣rep␣0␣plain␣x2␣tone |
| typo-0914 | `w00␣` | 攤 | w00␣ | final-repeat-combo␣rep␣0␣plain␣x2␣space |
| typo-0919 | `ru004` | 建 | ru004 | final-repeat-combo␣rep␣0␣medial␣x2␣tone |
| typo-0920 | `ru0004` | 建 | ru0004 | final-repeat-combo␣rep␣0␣medial␣x3␣tone |
| typo-0923 | `ru00␣` | 兼 | ru00␣ | final-repeat-combo␣rep␣0␣medial␣x2␣space |
| typo-0924 | `ru000␣` | 兼 | ru000␣ | final-repeat-combo␣rep␣0␣medial␣x3␣space |
| typo-0930 | `hkgl6` | hkg熬 | hkgl6 | final-repeat-combo␣combo␣hkgl6 |
| typo-0931 | `ckgl6` | ckg熬 | ck招 | final-repeat-combo␣combo␣ckgl6 |
| typo-0932 | `hkg;4` | hkg盎 | hk上 | final-repeat-combo␣combo␣hkg;4 |
| typo-0933 | `zsuj4` | 怒 | zsuj4 | final-repeat-combo␣combo␣zsuj4 |
| typo-0934 | `gsuj4` | g怒 | gsuj4 | final-repeat-combo␣combo␣gsuj4 |
| typo-0935 | `hhkkgg6` | hhkkg時 | hhkkgg6 | final-repeat-combo␣combo␣hhkkgg6 |
| typo-0936 | `qqwwuu//6` | 停 | qqwwuu//6 | final-repeat-combo␣combo␣qqwwuu//6 |
| typo-0937 | `zak7` | 麼 | zak7 | final-repeat-combo␣combo␣zak7 |
| typo-0938 | `sxk7` | 了 | sxk7 | final-repeat-combo␣combo␣sxk7 |
| typo-0939 | `dxk7` | 了 | dxk7 | final-repeat-combo␣combo␣dxk7 |
| typo-0940 | `vak7` | v麼 | vak7 | final-repeat-combo␣combo␣vak7 |
| typo-0941 | `qwu0␣` | 天 | qwu0␣ | final-repeat-combo␣combo␣qwu0␣ |
| typo-0942 | `xwu0␣` | x天 | xwu0␣ | final-repeat-combo␣combo␣xwu0␣ |
| typo-0943 | `hkg3` | 捨 | hkg3 | final-repeat-combo␣combo␣hkg3 |
| typo-0944 | `ftg;4` | 上 | ft上 | final-repeat-combo␣combo␣ftg;4 |
| typo-0946 | `cfg;4` | c上 | cf上 | final-repeat-combo␣combo␣cfg;4 |
| typo-0947 | `ji3qwu0␣` | 我天 | 我qwu0␣ | final-repeat-combo␣combo␣ji3qwu0␣ |
| typo-0948 | `qwu0␣ji3` | 天我 | qwu0␣我 | final-repeat-combo␣combo␣qwu0␣ji3 |
| typo-0950 | `1qu/6` | 1平 | 1坪 | digit-classifier␣cls␣1␣qu/6 |
| typo-0953 | `1q84` | 怕 | 1怕 | digit-classifier␣non␣1␣q84 |
| typo-0954 | `1ql3` | 跑 | 1跑 | digit-classifier␣non␣1␣ql3 |
| typo-0955 | `1q/6` | 朋 | 1朋 | digit-classifier␣non␣1␣q/6 |
| typo-0956 | `1q;4` | 胖 | 1胖 | digit-classifier␣non␣1␣q;4 |
| typo-0958 | `2qu/6` | 2平 | 2坪 | digit-classifier␣cls␣2␣qu/6 |
| typo-0961 | `2q84` | 怕 | 2怕 | digit-classifier␣non␣2␣q84 |
| typo-0962 | `2ql3` | 跑 | 2跑 | digit-classifier␣non␣2␣ql3 |
| typo-0963 | `2q/6` | 朋 | 2朋 | digit-classifier␣non␣2␣q/6 |
| typo-0964 | `2q;4` | 胖 | 2胖 | digit-classifier␣non␣2␣q;4 |
| typo-0969 | `1w;4` | 1燙 | 1趟 | digit-classifier␣cls␣1␣w;4 |
| typo-0970 | `1w94` | 太 | 1大 | digit-classifier␣non␣1␣w94 |
| typo-0971 | `1wj/4` | 痛 | 1同 | digit-classifier␣non␣1␣wj/4 |
| typo-0972 | `1w06` | 潭 | 1彈 | digit-classifier␣non␣1␣w06 |
| typo-0973 | `1w8␣` | 它 | 1w8␣ | digit-classifier␣non␣1␣w8␣ |
| typo-0978 | `2w;4` | 2燙 | 2趟 | digit-classifier␣cls␣2␣w;4 |
| typo-0979 | `2w94` | 太 | 2大 | digit-classifier␣non␣2␣w94 |
| typo-0980 | `2wj/4` | 痛 | 2同 | digit-classifier␣non␣2␣wj/4 |
| typo-0981 | `2w06` | 潭 | 2彈 | digit-classifier␣non␣2␣w06 |
| typo-0982 | `2w8␣` | 它 | 2w8␣ | digit-classifier␣non␣2␣w8␣ |
| typo-0983 | `2ek4` | 各 | 2個 | digit-classifier␣cls␣2␣ek4 |
| typo-0984 | `2ep␣` | 2跟 | 2ep␣ | digit-classifier␣cls␣2␣ep␣ |
| typo-0985 | `2ej04` | 2灌 | 2罐 | digit-classifier␣cls␣2␣ej04 |
| typo-0986 | `2ej3` | 2古 | 2股 | digit-classifier␣cls␣2␣ej3 |
| typo-0987 | `2el␣` | 高 | 2el␣ | digit-classifier␣non␣2␣el␣ |
| typo-0988 | `2eo3` | 給 | 2給 | digit-classifier␣non␣2␣eo3 |
| typo-0989 | `2e03` | 感 | 2桿 | digit-classifier␣non␣2␣e03 |
| typo-0990 | `2e.3` | 狗 | 2狗 | digit-classifier␣non␣2␣e.3 |
| typo-0991 | `5ru04` | 5建 | 5件 | digit-classifier␣cls␣5␣ru04 |
| typo-0992 | `5ru8␣` | 5家 | 5ru8␣ | digit-classifier␣cls␣5␣ru8␣ |
| typo-0993 | `5rm4` | 5句 | 5巨 | digit-classifier␣cls␣5␣rm4 |
| typo-0994 | `5rup␣` | 5金 | 5rup␣ | digit-classifier␣cls␣5␣rup␣ |
| typo-0995 | `5rul4` | 叫 | 5教 | digit-classifier␣non␣5␣rul4 |
| typo-0996 | `5ru.3` | 久 | 5九 | digit-classifier␣non␣5␣ru.3 |
| typo-1001 | `5t3` | 5尺 | 5呎 | digit-classifier␣cls␣5␣t3 |
| typo-1002 | `5tj4` | 5畜 | 5處 | digit-classifier␣cls␣5␣tj4 |
| typo-1003 | `5t␣` | 吃 | 5t␣ | digit-classifier␣non␣5␣t␣ |
| typo-1004 | `5t86` | 查 | 5查 | digit-classifier␣non␣5␣t86 |
| typo-1005 | `5tl3` | 吵 | 5炒 | digit-classifier␣non␣5␣tl3 |
| typo-1006 | `5tj;␣` | 窗 | 5tj;␣ | digit-classifier␣non␣5␣tj;␣ |
| typo-1007 | `5yji4` | 5做 | 5座 | digit-classifier␣cls␣5␣yji4 |
| typo-1011 | `5y94` | 在 | 5在 | digit-classifier␣non␣5␣y94 |
| typo-1012 | `5y.3` | 走 | 5走 | digit-classifier␣non␣5␣y.3 |
| typo-1013 | `5yl3` | 早 | 5早 | digit-classifier␣non␣5␣yl3 |
| typo-1014 | `5yjo4` | 最 | 5最 | digit-classifier␣non␣5␣yjo4 |
| typo-1016 | `5w94` | 5太 | 5大 | digit-classifier␣far␣5w94 |
| typo-1017 | `2rul4` | 2叫 | 2教 | digit-classifier␣far␣2rul4 |
| typo-1018 | `1t␣` | 1吃 | 1t␣ | digit-classifier␣far␣1t␣ |
| typo-1021 | `x9s06` | x9難 | x9南 | combo-2␣combo2␣x9s06 |
| typo-1023 | `hkdl3` | h考 | hk考 | combo-2␣combo2␣hkdl3 |
| typo-1024 | `wlq;6` | 旁 | wlq;6 | combo-3␣combo3␣wlq;6 |
| typo-1025 | `wpq/6` | 朋 | wp澎 | combo-3␣combo3␣wpq/6 |
| typo-1026 | `c.d04` | c.看 | c.d04 | combo-3␣combo3␣c.d04 |
| typo-1027 | `x/s06` | x/難 | x/s06 | combo-3␣combo3␣x/s06 |
| typo-1028 | `hpgl3` | 少 | hp少 | combo-3␣combo3␣hpgl3 |
| typo-1029 | `flg/4` | 剩 | fl勝 | combo-3␣combo3␣flg/4 |
| typo-1030 | `clq;6` | c旁 | cl龐 | combo-3␣combo3␣clq;6 |
| typo-1031 | `w8q84` | 怕 | w8怕 | combo-3␣combo3␣w8q84 |
| typo-1033 | `wlql3` | 跑 | wl跑 | combo-3␣combo3␣wlql3 |

</details>

<details><summary>mixed-habit1：錯 39 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| mx1-002 | `au/6wu0␣2k7␣meeting␣e932l4␣3␣2u03{Ctrl+,}ru42k6294␣PPT{Ctrl+.}` | 明天的␣meeting␣改到␣3␣點，記得帶␣PPT。 | 明天的␣meeting␣改道␣3␣點，記得帶␣PPT。 |  |
| mx1-006 | `5k4w961u32u04g4␣i7␣ru8␣␣16GB␣ru4u4wu3{Ctrl+,}vu,3t/6g4e.4m/4xk7{Ctrl+.}` | 這台筆電是␣i7␣加␣16GB␣記憶體，寫程式夠用了。 | 這台筆電是␣喔␣加␣16GB␣記憶體，寫程式夠用了。 |  |
| mx1-007 | `fu/3183cjo4u4ru4xj45j03t/6␣pdf␣y94ru4eo3ji3{Ctrl+.}` | 請把會議紀錄轉成␣pdf␣再寄給我。 | 請把會議紀錄轉成␣pdf␣在寄給我。 |  |
| mx1-012 | `j03g;4␣7␣2u03y94vup4u4fm␣ru04{Ctrl+,}1j4ul4t62l4i␣{Ctrl+.}` | 晚上␣7␣點在信義區見，不要遲到喔。 | 晚上␣7␣點在信義區健，不要遲到喔。 |  |
| mx1-013 | `5k45;␣␣jpg␣2;3w94284xk7{Ctrl+,}u8␣2l4␣2MB␣u3vu84y94g;4tj06{Ctrl+.}` | 這張␣jpg␣檔太大了，壓到␣2MB␣以下再上傳。 | 這張␣ㄕㄨㄣ檔太大了，壓到␣2MB␣以下再上傳。 |  |
| mx1-017 | `5k45␣␣API␣cjo6tj062k7ek6g4g4␣JSON{Ctrl+,}1j4g4␣XML{Ctrl+.}` | 這支␣API␣回傳的格式是␣JSON，不是␣XML。 | 這隻␣API␣回傳的格式是␣JSON，不是␣XML。 |  |
| mx1-018 | `vu84y93xk7␣mp3␣204g4g.3ru␣1i41j4tj␣x96{Ctrl+.}` | 下載了␣mp3␣但是手機播不出來。 | 下載了␣允␣但是手機播不出來。 |  |
| mx1-024 | `j3h0␣u␣zp4␣120␣m06{Ctrl+,}1u3g;4ek4m,4ejo4xk7␣10␣dj94{Ctrl+.}` | 午餐一份␣120␣元，比上個月貴了␣10␣塊。 | 午餐一份␣120␣元，比上個月貴了␣斑塊。 |  |
| mx1-026 | `w8␣2k7␣YouTube␣qup62l4u3ru/␣u.3␣50␣j042u/4m,4xk7{Ctrl+.}` | 他的␣YouTube␣頻道已經有␣50␣萬訂閱了。 | 他的␣YouTube␣頻道已經有␣詹萬訂閱了。 |  |
| mx1-027 | `ru42k6vu0␣␣git␣pull␣y94e93{Ctrl+,}1j4b06cjo4␣conflict{Ctrl+.}` | 記得先␣git␣pull␣再改，不然會␣conflict。 | 記得先␣git␣pull␣在改，不然會␣conflict。 |  |
| mx1-028 | `cjo4u4g4y94␣12␣x.6{Ctrl+,}2u04wu␣tj␣x96u.45j03{Ctrl+.}` | 會議室在␣12␣樓，電梯出來右轉。 | 會議是在␣12␣樓，電梯出來右轉。 |  |
| mx1-030 | `ji3vu0␣␣demo␣u␣h4{Ctrl+,}u.3jp4wu6y94wu6tj␣x96{Ctrl+.}` | 我先␣demo␣一次，有問題再提出來。 | 我先␣demo␣一次，有問題在提出來。 |  |
| mx1-034 | `2;304w94284ru41j4tj␣fm4{Ctrl+,}ji3z;42l4␣Google␣Drive␣eo3su3{Ctrl+.}` | 檔案太大寄不出去，我放到␣Google␣Drive␣給你。 | 檔案太大季不出去，我放到␣Google␣Drive␣給你。 |  |
| mx1-048 | `s84ek7u/3qu0453u.3␣720p{Ctrl+,}d04fu3x96cl3cj6{Ctrl+.}` | 那個影片只有␣720p，看起來好糊。 | 那個影片只有␣720p，看起來好胡。 |  |
| mx1-056 | `fu/3jp45k4ek7␣USB␣njo6gp␣2u,6{Ctrl+'}HDMI␣vu04g4go62k7{Ctrl+?}` | 請問這個␣USB␣隨身碟、HDMI␣線是誰的？ | 請問這個␣USB␣隨身碟、HDMI␣縣市誰的？ |  |
| mx1-061 | `yl3g;4␣8␣2u031042k7el␣wu,3{Ctrl+,}ji3␣7␣2u03ru.4ul4tj␣ap6{Ctrl+.}` | 早上␣8␣點半的高鐵，我␣7␣點就要出門。 | 早上␣阿點半的高鐵，我␣7␣點就要出門。 |  |
| mx1-065 | `vup␣2k7␣Pixel␣9␣q9␣5l45p␣2k7cp3fu;6{Ctrl+.}` | 新的␣Pixel␣9␣拍照真的很強。 | 新的␣Pixel␣埃拍照真的很強。 |  |
| mx1-067 | `w8␣yji6wu0␣l6u,4283␣LOL␣2l4yl3g;4␣5␣2u03{Ctrl+.}` | 他昨天熬夜打␣LOL␣到早上␣5␣點。 | 他昨天熬夜打␣LOL␣到早上␣支點。 |  |
| mx1-075 | `hi{Ctrl+,}fu/3jp4au/6wu0␣2k7dk4c96u.3au/6k6a87{Ctrl+?}` | hi，請問明天的課還有名額嗎？ | hi，請問明天的克還有名額嗎？ |  |
| mx1-076 | `ji32k7vup4vu;␣{Ctrl+:}test@gmail.com{Ctrl+,}u.3jp4wu6y94vu,3vup4eo3ji3{Ctrl+.}` | 我的信箱：test@gmail.com，有問題再寫信給我。 | 我的信箱：test@gmail.com，有問題在寫信給我。 |  |
| mx1-077 | `su32k7␣GitHub␣5;4cl4g4gp6ak7{Ctrl+?}ji3ru8␣su3rup4␣repo{Ctrl+.}` | 你的␣GitHub␣帳號是什麼？我加你進␣repo。 | 你的␣GitHub␣帳號是什麼？我家你進␣repo。 |  |
| mx1-081 | `ji31835l4qu04h96t/6␣16:9␣2k71u3xu4{Ctrl+.}` | 我把照片裁成␣16:9␣的比例。 | 我把照片才成␣16:9␣的比例。 |  |
| mx1-082 | `5k4ek7wl4ru04y94␣Python␣3.12␣5j;␣1j4fu3x96{Ctrl+,}cl3vu;41031p3tj/␣wj6{Ctrl+.}` | 這個套件在␣Python␣3.12␣裝不起來，好像版本衝突。 | 這個套件在␣Python␣3.12␣莊不起來，好像版本衝突。 |  |
| mx1-084 | `w8␣gji␣1l4ru84ul4c06␣5%␣2k7gjo4{Ctrl+.}` | 他說報價要含␣5%␣的稅。 | 他說報價要韓␣5%␣的稅。 |  |
| mx1-085 | `ji3183␣Chrome␣2k7zp␣u,4d9␣2l4␣50␣ek4{Ctrl+,}2u04sl3dj942;4ru␣xk7{Ctrl+.}` | 我把␣Chrome␣的分頁開到␣50␣個，電腦快當機了。 | 我把␣Chrome␣的分頁開到␣詹個，電腦快當機了。 |  |
| mx1-089 | `cjo4u4g42k7w.6u/3ru␣53u.3␣1080p{Ctrl+,}y4d041j4w94fu/␣tj3{Ctrl+.}` | 會議室的投影機只有␣1080p，字看不太清楚。 | 會議室的投影機只有␣1080p，自看不太清楚。 |  |
| mx1-090 | `vu84j3␣2␣2u03104u.3␣1␣on␣1{Ctrl+,}dk3s/6cjo4j032u03cjo6{Ctrl+.}` | 下午␣2␣點半有␣1␣on␣1，可能會晚點回。 | 下午␣2␣點半有␣1␣塞1，可能會晚點回。 |  |
| mx1-093 | `5k45;␣5l4qu04dk3u35j03t/6␣png␣a87{Ctrl+?}ul4fm41o4{Ctrl+.}` | 這張照片可以轉成␣png␣嗎？要去背。 | 這張照片可以轉成␣png␣嗎？要去被。 |  |
| mx1-103 | `Apple␣Watch␣wu6vu/3ji3rup␣wu0␣y.31j42l4␣5000␣1j4{Ctrl+.}` | Apple␣Watch␣提醒我今天走不到␣5000␣步。 | Apple␣Watch␣提醒我今天走不到␣5000␣不。 |  |
| mx1-110 | `w.6u/3qu042u4␣5␣u,42k7wj61ul3ul4e/4vup␣gj4rm4{Ctrl+.}` | 投影片第␣5␣頁的圖表要更新數據。 | 投影片第␣之夜的圖表要更新數據。 |  |
| mx1-119 | `2/3u␣vu84␣5␣zp␣5j/␣{Ctrl+,}ji3ql4ek4d8␣zo␣{Ctrl+.}` | 等一下␣5␣分鐘，我泡個咖啡。 | 等一下␣之分鐘，我泡個咖啡。 |  |
| mx1-124 | `ji3183cjo4u4e932l4␣Google␣Meet{Ctrl+,}xu06ru,6y94vu/6g4xu4g;4{Ctrl+.}` | 我把會議改到␣Google␣Meet，連結在行事曆上。 | 我把會議改道␣Google␣Meet，連結在行事曆上。 |  |
| mx1-127 | `cl32k7{Ctrl+,}ji3␣10␣zp␣5j/␣c.42l4{Ctrl+.}` | 好的，我␣10␣分鐘後到。 | 好的，我␣班分鐘後到。 |  |
| mx1-136 | `su3dk3u3vu0␣m/4␣Postman␣hk4g4u␣vu845k45␣␣API{Ctrl+.}` | 你可以先用␣Postman␣測試一下這支␣API。 | 你可以先用␣Postman␣測試一下這隻␣API。 |  |
| mx1-139 | `5k4dj03␣SUV␣u.6cl4284e94␣15␣ej/␣xu3u␣ej/␣g/␣{Ctrl+.}` | 這款␣SUV␣油耗大概␣15␣公里一公升。 | 這款␣SUV␣游顥大概␣15␣公里一公升。 |  |
| mx1-144 | `ao3wu0␣2k7294104g4vu;4ji32.␣ru4y94␣Todoist␣g;4{Ctrl+.}` | 每天的待辦事項我都記在␣Todoist␣上。 | 每天的待辦事項我都記載␣Todoist␣上。 |  |
| mx1-145 | `vu04g;4dk4t/6283␣8␣5k6{Ctrl+,}53ul4␣2400␣m06{Ctrl+.}` | 線上課程打␣8␣折，只要␣2400␣元。 | 線上課程打␣阿哲，只要␣2400␣元。 |  |
| mx1-147 | `ji32k7␣IG␣5;4cl41o42l4xk7{Ctrl+,}e03dj94d9␣fu3␣2FA{Ctrl+.}` | 我的␣IG␣帳號被盜了，趕快開啟␣2FA。 | 我的␣IG␣帳號被到了，趕快開啟␣2FA。 |  |
| mx1-149 | `bye{Ctrl+,}au/6wu0␣ej/␣n␣ru04{Ctrl+.}` | bye，明天公司見。 | bye，明天公司建。 |  |

</details>

<details><summary>mixed-habit2：錯 55 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| mx2-001 | `ji3vu84y93xk7mp3ek6g4␣204g41i41j4tj␣x96` | 我下載了mp3格式␣但是播不出來 | 我下載了允格式␣但是播不出來 |  |
| mx2-002 | `3nm54t/62k7ru/␣qu04cl3ejo4␣2/3au/6su06y94a93cl3xk7` | 3nm製程的晶片好貴␣等明年再買好了 | 3nm54成的晶片好貴␣等明年再買好了 |  |
| mx2-010 | `4ku/6aj4d04u/3qu045p␣2k7t8␣cp32ji␣` | 4k螢幕看影片真的差很多 | 4ku/6木看影片真的差很多 |  |
| mx2-012 | `au/6wu0␣92u03ul4rul␣1l4el4␣rup␣j03u.4ul4l6u,4xk7` | 明天9點要交報告␣今晚又要熬夜了 | 明天92演要交報告␣今晚又要熬夜了 |  |
| mx2-020 | `vup␣2k7macbookcl3fu/␣␣294tj␣ap6cp3z;␣1u04` | 新的macbook好輕␣帶出門很方便 | 新的macbook好清␣帶出門很方便 |  |
| mx2-023 | `5k41j4u/3qu0453u.3480pcj8456␣ep␣1p3d041j4fu/␣tj3` | 這部影片只有480p畫質␣根本看不清楚 | 這部影片只有480pcj8456␣根本看不清楚 |  |
| mx2-030 | `2/32/352u03vu8410␣␣ul41j4ul4fm4t␣cji3eji␣` | 等等5點下班␣要不要去吃火鍋 | 等等52眼下班␣要不要去吃火鍋 |  |
| mx2-033 | `5k4w961u32u04g4i72k7␣ql3t/6g4cp3dj94` | 這台筆電是i7的␣跑程式很快 | 這台筆電是喔的␣跑程式很快 |  |
| mx2-036 | `vu84y93xk7u␣ek7100GB2k7u.6vu4␣ql3xk7u␣5/3j03` | 下載了一個100GB的遊戲␣跑了一整晚 | 下載了一個100GB2k7u.6vu4␣跑了一整晚 |  |
| mx2-037 | `5gy94g0␣g;4ep␣1p3g.␣1j42l4vmp4cl4` | 5g在山上根本收不到訊號 | 5gy94g0␣上根本收不到訊號 |  |
| mx2-039 | `2/3ji320zp␣5j/␣␣ji3a83g;42l4` | 等我20分鐘␣我馬上到 | 等我20zp␣中␣我馬上到 |  |
| mx2-041 | `5k45;␣jpgw94284xk7␣tj061j4g;4fm4` | 這張jpg太大了␣傳不上去 | 這張jpgw94284了␣船不上去 |  |
| mx2-043 | `120hz2k7u/6aj4cj86fu3x96cl3gjp4` | 120hz的螢幕滑起來好順 | 120hz2k7螢幕滑起來好順 |  |
| mx2-044 | `ji32k7ipadtj/␣1j4xk72u04␣g41j4g4vu04cj94xk7` | 我的ipad充不了電␣是不是線壞了 | 我的ipadtj/␣不了電␣是不是現壞了 |  |
| mx2-045 | `cl3x8␣byeau/6wu0␣ej/␣n␣ru04` | 好啦bye明天公司見 | 好啦bye明天公司建 |  |
| mx2-048 | `rup␣wu0␣deploy2l45/4g4ru␣␣ru,6eji3tj␣1l␣xk7` | 今天deploy到正式機␣結果出包了 | 今天deploy2奧正式機␣結果出包了 |  |
| mx2-049 | `ji3y94vu8␣qu6a93xk7u␣wul6usbvu04␣2/3xk7u␣ek7xu3194h962l4` | 我在蝦皮買了一條usb線␣等了一個禮拜才到 | 我在蝦皮買了一條usbvu04␣等了一個禮拜才到 |  |
| mx2-052 | `su32k7gmailg4gp6ak7␣ji3ru4ul␣fu/3eo3su3` | 你的gmail是什麼␣我寄邀請給你 | 你的gmailg4什麼␣我既邀請給你 |  |
| mx2-054 | `ji3ru8␣2k7j;3xj4g4500m2k7␣283u.6vu42.␣1j4cjo4lag` | 我家的網路是500m的␣打遊戲都不會lag | 我家的網路是500m2k7␣打遊戲都不會lag |  |
| mx2-056 | `yl3g;482u032k7cjo4ji3u␣2u/4fu31j4x96` | 早上8點的會我一定起不來 | 早上82演的會我一定起不來 |  |
| mx2-059 | `su3cpujp␣2j4w94el␣xk7␣ul41j4ul4fu/␣u␣vu84cjo␣tp6` | 你cpu溫度太高了␣要不要清一下灰塵 | 你cpujp␣度太高了␣要不要清一下灰塵 |  |
| mx2-062 | `w8␣ao3wu0␣2.␣y94igvu042j/4g;3z8␣ao3g6` | 她每天都在ig限動上發美食 | 他每天都在ig限動上發美食 |  |
| mx2-065 | `u/42u,653g/42gbdj/␣ru0␣␣gp6ak72.␣5j;␣1j4xk7` | 硬碟只剩2gb空間␣什麼都裝不了 | 硬碟只剩2gbdj/␣間␣什麼都裝不了 |  |
| mx2-066 | `tj␣ap6ru42k6294tj/␣2u04fu4ep␣type-c2k7vu04` | 出門記得帶充電器跟type-c的線 | 出門記得帶充電器跟type-c2k7線 |  |
| mx2-075 | `e/4vup␣j06win115␣c.4u␣56x06qu/6␣53cl3tj/6ej04` | 更新完win11之後一直藍屏␣只好重灌 | 更新完win115␣後一直藍瓶␣只好重灌 |  |
| mx2-076 | `ji3y94uber␣eats2u03xk7u␣1o␣5p␣s93` | 我在uber␣eats點了一杯珍奶 | 我在uber␣eats2演了一杯珍奶 |  |
| mx2-082 | `ji3rm,62k6excel1u3googleg4nj041ul3cl3m/4` | 我覺得excel比google試算表好用 | 我覺得excel1以google試算表好用 |  |
| mx2-083 | `5k4h4dl3g4dl3xk798zp␣␣tl␣d9␣vup␣` | 這次考試考了98分␣超開心 | 這次考試考了98zp␣␣超開心 |  |
| mx2-084 | `w8␣a93xk7tesla5␣c.4␣ao3wu0␣2.␣y94vm04ul4` | 他買了tesla之後␣每天都在炫耀 | 他買了tesla5␣後␣每天都在炫耀 |  |
| mx2-086 | `rup␣wu0␣ru8␣10␣2l4102u03␣cl3xo4` | 今天加班到10點␣好累 | 今天加班到102演␣好累 |  |
| mx2-087 | `vpnu␣562j04vu04␣ep␣1p3ao6104z83m032j0␣ej/␣yji4` | vpn一直斷線␣根本沒辦法遠端工作 | vpnu␣直斷線␣根本沒辦法遠端工作 |  |
| mx2-091 | `5k45.␣ai4ul41j4ul4fm4dp32u/␣␣ji3yj␣u␣w96suvd9␣eji4fm4` | 這週末要不要去墾丁␣我租一台suv開過去 | 這週末要不要去墾丁␣我租一台suvd9␣過去 |  |
| mx2-102 | `vul3c96ao3wu0␣d04youtubed042l41j4vu;3gjo4` | 小孩每天看youtube看到不想睡 | 小孩每天看youtubed04到不想睡 |  |
| mx2-109 | `5k45␣g.3ru␣5␣m0665wdj94tj/␣␣104vul3g6ru.4tj/␣a03` | 這支手機支援65w快充␣半小時就充滿 | 這支手機支援65wdj94tj/␣␣半小時就充滿 |  |
| mx2-110 | `5k41j42u04u/3g43d2k7␣ul4xu/4j94294u03ru/4` | 這部電影是3d的␣要另外戴眼鏡 | 這部電影是3d2k7␣要另外戴眼鏡 |  |
| mx2-115 | `ji3y947-112/3su3␣su32l4xk7283eo3ji3` | 我在7-11等你␣你到了打給我 | 我在7-112/3你␣你到了打給我 |  |
| mx2-118 | `5k4ek7cssy94safarig;4cjo4ql3103` | 這個css在safari上會跑版 | 這個cssy94safarig;4會跑版 |  |
| mx2-120 | `su35␣2l4c++ep␣javas83ek71u3rul4s06a87` | 你知道c++跟java哪個比較難嗎 | 你知道c++ep␣java哪個比較難嗎 |  |
| mx2-121 | `ji31l4au/6xk72ji␣u4␣aj41ul␣800zp␣u3g;4` | 我報名了多益␣目標800分以上 | 我報名了多益␣目標800zp␣以上 |  |
| mx2-122 | `ji32k7iclouddj/␣ru0␣a03xk7␣5l4qu042.␣1o4zp41j4xk7` | 我的icloud空間滿了␣照片都備份不了 | 我的iclouddj/␣間滿了␣照片都備份不了 |  |
| mx2-124 | `ji32/32/3ul4fm4fm06ru8␣s861l␣eji3␣ul41;␣su3294starbucksa87` | 我等等要去全家拿包裹␣要幫你帶starbucks嗎 | 我等等要去全家拿包裹␣要幫你帶starbucksa87 |  |
| mx2-127 | `5k4ek7svgwj6g4z;4284u,31j4cjo4cj6` | 這個svg圖示放大也不會糊 | 這個svg圖是放大也不會胡 |  |
| mx2-129 | `rup␣wu0␣2k7stand-upwji␣xk7104vul3g6` | 今天的stand-up拖了半小時 | 今天的stand-upwji␣了半小時 |  |
| mx2-130 | `5k4h4tj␣t9␣y94zo␣ru␣g;4m/4kindled04j06u␣1p3gj␣` | 這次出差在飛機上用kindle看完一本書 | 這次出差在飛機上用kindled按完一本書 |  |
| mx2-133 | `x/3fu4d9␣262j4e;␣cl3␣w94x/3cjo4e03al4` | 冷氣開26度剛好␣太冷會感冒 | 冷氣開262物剛好␣太冷會感冒 |  |
| mx2-134 | `ji31835ji␣1j4cj04t/68k2k7z/␣ru/35l4` | 我把桌布換成8k的風景照 | 我把桌布換成8k2k7風景照 |  |
| mx2-135 | `ji3tj062k7pptxsu3u.3g.␣2l4a87␣ji3e93xk72u45u,4` | 我傳的pptx你有收到嗎␣我改了第5頁 | 我船的pptx你有收到嗎␣我改了第5頁 |  |
| mx2-137 | `xl3103gji␣su062u3wul6vup␣5%-6u3␣cl3g␣j;4` | 老闆說年底調薪5%而已␣好失望 | 老闆說年底調薪5%-6以␣好失望 |  |
| mx2-138 | `ji3ap762u03104y94ru,6mp4504tj␣d.3ru04` | 我們6點半在捷運站出口見 | 我們6點半在捷運站出口健 |  |
| mx2-139 | `s84ek7wl4ru04g/␣ru62l4v2.15␣c.4ru.45/4t;6xk7` | 那個套件升級到v2.1之後就正常了 | 那個套件升級到v2.15␣後就正常了 |  |
| mx2-142 | `ji3vu;3a93dysonvu␣tp6fu4␣dk3g4ul42j042ji␣` | 我想買dyson吸塵器␣可是要2萬多 | 我想買dysonvu␣陳氣␣可是要斷多 |  |
| mx2-143 | `rup␣wu0␣2k71l4el4m/4xk7205;␣w.6u/3qu04␣ru;32l4d.3e0␣gk6yl4` | 今天的報告用了20張投影片␣講到口乾舌燥 | 今天的報告用了205;␣投影片␣講到口乾舌燥 |  |
| mx2-145 | `vu04g;4dk4t/628385k6␣vu04y9453ul41999` | 線上課程打8折␣現在只要1999 | 線上課程打85哦␣現在只要1999 |  |
| mx2-146 | `5k4183ru␣vu,4ru04q06g4fu/␣5.62k7␣283code2k7g6c.4tl␣tl3` | 這把機械鍵盤是青軸的␣打code的時候超吵 | 這把機械鍵盤是輕軸的␣打code的時候超吵 |  |
| mx2-148 | `e;␣e;␣wifi2j042ul4␣ji3m/4g.3ru␣bk42u03t/␣u␣vu84` | 剛剛wifi斷掉␣我用手機熱點撐一下 | 剛剛wifi2j042ul4␣我用手機熱點撐一下 |  |

</details>

<details><summary>spec：錯 34 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| mix-03 | `3nm54t/6` | 3nm製程 | 3nm54成 | P7；官方␣3nm54成 |
| mix-04 | `4ku/6aj4` | 4k螢幕 | 4ku/6木 | P7；官方␣4ku/6木 |
| mix-05 | `ji3vu84y94xk7mp3ek6g4` | 我下載了mp3格式 | 我下在了隕格式 | P7；官方␣我下在了隕格式 |
| mix-06 | `m4nj0453e.4i31u3rul4ejo4` | 預算只夠i3比較貴 | 預算只夠嚄比較貴 | P7；官方␣預算只夠嚄比較貴 |
| mix-11 | `npm␣` | npm | ㄙㄩㄣ | P5；官方␣ㄙㄩㄣ |
| mix-12 | `cpu␣` | cpu | ㄏㄧㄣ | P5；官方␣ㄏㄧㄣ |
| mix-13 | `nm{Enter}` | nm | ㄙㄩ | P5；官方送出␣ㄙㄩ |
| mix-15 | `e(` | e( | ㄍ( | P5；官方␣ㄍ（ |
| mix-16 | `e{KP2}e` | e2e | ㄍ2e | P5；官方␣ㄍ2e |
| red-01 | `wuuuu/6` | 停 | wuuu營 | P6；官方␣wuuu營 |
| red-02 | `wu//6` | 停 | wu//6 | P6 |
| red-03 | `wuwu/6` | 停 | wu停 | P6 |
| red-04 | `ggggk6` | 什 | ggg什 | P6 |
| red-05 | `ji3wuuuu/6su3` | 我停你 | 我wuuu贏你 | P6 |
| red-06 | `hellowu//6` | hello停 | hellowu//6 | P6 |
| red-07 | `wu//␣` | 聽 | wu//␣ | P6：結尾是空白也合併 |
| red-08 | `ssl␣` | 孬 | ssl␣ | P6：英文字典沒收的會變字 |
| red-09 | `ru8884` | 架 | ru8884 | P6：有介音時數字鍵＋聲調␣4␣個以內合併 |
| red-11 | `gkgk6` | 什 | gk什 | P6；華碩␣gkg額，不照抄 |
| typo-01 | `hkg6` | 什 | hkg6 | P6：相鄰的聲母丟掉 |
| typo-02 | `ckg6` | c什 | ck時 | P6：不相鄰的切成英文 |
| typo-03 | `qwu0␣` | 天 | qwu0␣ | P6 |
| typo-04 | `ji3rup␣qwu0␣cp3d9␣vup␣` | 我今天很開心 | 我今qwu0␣很開心 | P6；官方␣我今qwu0␣很開心 |
| typo-05 | `sju3` | 你 | sju3 | P6：介音後按的蓋掉前面的 |
| typo-06 | `sujm3` | 女 | sujm3 | P6 |
| typo-07 | `g8k4` | 設 | g8俄 | P6：韻母後按的蓋掉前面的 |
| typo-08 | `cip3` | 很 | cip3 | P6 |
| typo-10 | `1qu0␣` | 1篇 | 1qu0␣ | P6：數字一律留數字 |
| typo-14 | `5jiy6` | 昨 | 5jiy6 | P6 |
| typo-15 | `gjn␣` | 蘇 | gjn␣ | P6 |
| sym-05 | `{Ctrl+[}` | 【 |  | P4；官方沒反應 |
| sym-06 | `{Ctrl+]}` | 】 |  | P4；官方沒反應 |
| sym-07 | `{Ctrl+[}{Down}2` | 「 | 2 | P4：照畫面順序選 |
| sym-10 | `ji3$wu0␣` | 我$天 | 我$wu0␣ | P4；官方␣我$wu0␣ |

</details>

<details><summary>zh-habit1：錯 28 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| zh1-001 | `rup␣wu0␣vu84j32k7cjo4u4e932l4n0␣2u03{Ctrl+,}ru42k62941u32u04{Ctrl+.}` | 今天下午的會議改到三點，記得帶筆電。 | 今天下午的會議改道三點，記得帶筆電。 |  |
| zh1-008 | `fu65j/␣dl32k7z04jo62l42u4j35;␣{Ctrl+,}xl3g␣gji␣1j4cjo4dl3zj4xj4{Ctrl+.}` | 期中考的範圍到第五章，老師說不會考附錄。 | 期中考的範圍到第五張，老師說不會考附錄。 |  |
| zh1-012 | `5k4ek7m,42k7vup␣gjo3bj45;4xk7{Ctrl+,}ji3vu0␣183z;6yj␣rul3u␣rul3{Ctrl+.}` | 這個月的薪水入帳了，我先把房租繳一繳。 | 這個月的薪水入帳了，我先把房租繳一角。 |  |
| zh1-019 | `up6c;6n0␣2u03104ru.4ej0␣ap6xk7{Ctrl+,}ul4104g42k6yl32u03tj␣ap6{Ctrl+.}` | 銀行三點半就關門了，要辦事得早點出門。 | 銀行三點半就關門了，要辦識得早點出門。 |  |
| zh1-030 | `28␣ru,6mp42l4w961o3tk␣504{Ctrl+,}y945j03ej/␣tk␣ru.42l4xk7{Ctrl+.}` | 搭捷運到台北車站，再轉公車就到了。 | 搭捷運到臺北車站，再轉公車就到了。 |  |
| zh1-032 | `u␣g/␣rul␣294ul4ul4z04c.4t␣{Ctrl+,}u␣wu0␣t␣n0␣h4{Ctrl+.}` | 醫生交代藥要飯後吃，一天吃三次。 | 醫生交代要要飯後吃，一天吃三次。 |  |
| zh1-034 | `5j3ej03vu␣j;4ji3ap7vu845.␣u␣fu06rul␣tj␣fu4cj84gj␣2k7tj␣el3{Ctrl+.}` | 主管希望我們下週一前交出企劃書的初稿。 | 主管希望我們下週一錢交出企劃書的初稿。 |  |
| zh1-037 | `u;␣c;6vm0␣1j4jo6t6xu4xm41j41u04{Ctrl+,}g4t;3z03u/4vu;␣2jo4qu/6jp3{Ctrl+.}` | 央行宣布維持利率不變，市場反應相對平穩。 | 央行宣佈維持利率不變，市場反應相對平穩。 |  |
| zh1-040 | `w8␣hj/6vul3ru.4vu3cj0␣u.6m/3{Ctrl+,}fm4su06c96s862l4fm06eji6ej04rmp␣{Ctrl+.}` | 她從小就喜歡游泳，去年還拿到全國冠軍。 | 他從小就喜歡游泳，去年還拿到全國冠軍。 |  |
| zh1-043 | `s/61j4s/6a86z06su3au/6wu0␣yl3g;41;␣ji3294ek410␣{Ctrl+?}` | 能不能麻煩你明天早上幫我代個班？ | 能不能麻煩你明天早上幫我帶個班？ |  |
| zh1-069 | `1u4u,42u03xu32u/4y94xu.4m,45j/␣{Ctrl+,}ru8␣5;3dk3u3u␣fu3h0␣ru8␣{Ctrl+.}` | 畢業典禮訂在六月中，家長可以一起參加。 | 畢業典禮定在六月中，家長可以一起參加。 |  |
| zh1-073 | `g45/4zj3ru4cj84y94au/6su06dji4ru045k4wul62l4xj4{Ctrl+.}` | 市政府計畫在明年擴建這條道路。 | 市政府計劃在明年擴建這條道路。 |  |
| zh1-113 | `w8␣xu06vm4n0␣su06283rup4g4ru,4m3fu.6rup31ul␣n942k7n4fu;6{Ctrl+.}` | 她連續三年打進世界羽球錦標賽的四強。 | 他連續三年打進世界羽球錦標賽的四強。 |  |
| zh1-118 | `ek61u42k7xup6rm␣cp3cl3vu;␣tj3{Ctrl+,}t;6t;6nj/4ji3ap7y4ru35j/32k7h94{Ctrl+.}` | 隔壁的鄰居很好相處，常常送我們自己種的菜。 | 隔壁的鄰居很好相處，常常送我們自己種的蔡。 |  |
| zh1-126 | `1j3vu610␣2k7dk4w942ji␣xk7{Ctrl+,}c96y7ru␣cj␣ao6u.3vu.␣vu62k7g6ru0␣{Ctrl+.}` | 補習班的課太多了，孩子幾乎沒有休息的時間。 | 補習班的客太多了，孩子幾乎沒有休息的時間。 |  |
| zh1-131 | `z;6294xu4xm4u.42ul4g/␣xk7{Ctrl+,}ao3ek7m,42k7zj420␣1u045j/41j4gl3{Ctrl+.}` | 房貸利率又調升了，每個月的負擔變重不少。 | 房貸利率又調升了，每個月的負擔變中不少。 |  |
| zh1-139 | `a8␣a8␣2jp4xk7n0␣ek7vul3g62k7ru␣w;␣{Ctrl+,}ck4fu3x96wk41u,6vu0␣wu06{Ctrl+.}` | 媽媽燉了三個小時的雞湯，喝起來特別鮮甜。 | 媽媽頓了三個小時的雞湯，喝起來特別鮮甜。 |  |
| zh1-141 | `fu6ru␣tk␣u␣2u/4ul42940␣fm06al4{Ctrl+,}vu84m3wu0␣e/4ul4z;4a04nj42j4{Ctrl+.}` | 騎機車一定要戴安全帽，下雨天更要放慢速度。 | 騎機車一定要帶安全帽，下雨天更要放慢速度。 |  |
| zh1-149 | `su065j/␣ru;3rup␣z8␣vu84x96xk7{Ctrl+,}283nj04s86u␣1j4zp4fm4xm3vu/6{Ctrl+.}` | 年終獎金發下來了，打算拿一部分去旅行。 | 年中獎金發下來了，打算拿一部分去旅行。 |  |
| zh1-152 | `5k4t;3j3w96rm42k7u03m061ul3vu042k6xup6xu6rup454{Ctrl+,}xu/4bp6up4vu;4gp␣dk4{Ctrl+.}` | 這場舞台劇的演員表現得淋漓盡致，令人印象深刻。 | 這場舞臺劇的演員表現得淋漓盡致，令人印象深刻。 |  |
| zh1-154 | `a86z06su3gjp4xj41;␣ji31835k42942j/␣vu␣s86eo3x.6vu842k7ej03xu3m06{Ctrl+.}` | 麻煩你順路幫我把這袋東西拿給樓下的管理員。 | 麻煩你順路幫我把這帶東西拿給樓下的管理員。 |  |
| zh1-159 | `fm4ru.3zp4u␣2u/4ul4t␣m4m06{Ctrl+,}y942l4t86x.6ck␣1o␣t86{Ctrl+.}` | 去九份一定要吃芋圓，再到茶樓喝杯茶。 | 去九份一定要吃芋圓，在到茶樓喝杯茶。 |  |
| zh1-165 | `vu4wj/3vu03g4sup62k7wjo4dj03u3tj3xu3j06t/6{Ctrl+,}fu/3xu.6u45;4cj4{Ctrl+.}` | 系統顯示您的退款已處理完成，請留意帳戶。 | 系統顯示您的退款以處理完成，請留意帳戶。 |  |
| zh1-169 | `w8␣ql3j06bp6g/␣2u4u␣t;3fm06t/6a83x8␣nj/␣{Ctrl+,}ru␣2j/42k6dj␣xk7{Ctrl+.}` | 她跑完人生第一場全程馬拉松，激動得哭了。 | 他跑完人生第一場全程馬拉松，激動得哭了。 |  |
| zh1-178 | `rup␣su062j/␣wu0␣wk41u,6jp␣sj03{Ctrl+,}2l4vu04y942.␣c96ao6tj0␣eji4284u␣{Ctrl+.}` | 今年冬天特別溫暖，到現在都還沒穿過大衣。 | 今年冬天特別溫暖，到現在都還沒穿過大一。 |  |
| zh1-190 | `fu/3jp45k4ek7yji4jo4u.3bp6yji4a87{Ctrl+?}` | 請問這個座位有人坐嗎？ | 請問這個座位有人做嗎？ |  |
| zh1-195 | `q/6cj62k7c93gjo3cl3fu/␣tk4{Ctrl+,}zj6fu06dk3u3d042l4cp32ji␣m6{Ctrl+.}` | 澎湖的海水好清澈，浮潛可以看到很多魚。 | 澎湖的海水好清澈，浮潛可以看到很多於。 |  |
| zh1-200 | `fu/3sup6vu0␣wu06vu,3gp␣fu/31ul3{Ctrl+(}5/4z03xu;3au042.␣ul4wu06{Ctrl+)}{Ctrl+,}y942l4-4cl4ejo4w96q962jo4{Ctrl+.}` | 請您先填寫申請表（正反兩面都要填），再到二號櫃台排隊。 | 請您先填寫申請表（正反兩面都要填），再到二號櫃檯排隊。 |  |

</details>

<details><summary>zh-habit2：錯 15 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| zh2-022 | `g;4h42k7fu06vu0␣c96su3␣g/4vu842k7m,42u3eo3` | 上次的錢先還你␣剩下的月底給 | 上次的前先還你␣剩下的月底給 |  |
| zh2-083 | `ru,3ru,3g/␣xk7␣g4ek7sm3g/␣␣cl3dk394` | 姊姊生了␣是個女生␣好可愛 | 姐姐生了␣是個女生␣好可愛 |  |
| zh2-089 | `t/6g4u.4tj␣hji4xk7␣5l3xk7u␣5/3wu0␣5l31j42l4m06up␣` | 程式又出錯了␣找了一整天找不到原因 | 城市又出錯了␣找了一整天找不到原因 |  |
| zh2-101 | `ru.4ru.4ru8␣y94w965j/␣␣eji4su062.␣cjo4fm4s841u0␣` | 舅舅家在台中␣過年都會去那邊 | 救救家在台中␣過年都會去那邊 |  |
| zh2-106 | `gj4vm,6u.4dl31j4ru6ek6␣cjo6ru8␣ul41o4su04xk7` | 數學又考不及格␣回家要被念了 | 數學又考不及格␣回家要被唸了 |  |
| zh2-107 | `su065j/␣dj94z8␣xk7␣vu␣j;4rup␣su062ji␣u␣2u03` | 年終快發了␣希望今年多一點 | 年中快發了␣希望今年多一點 |  |
| zh2-128 | `vu/6t/6ji3q96cl3xk7␣su3d04u␣vu84u.3ao6u.3ul4e93` | 行程我排好了␣你看一下有沒有要改 | 形成我排好了␣你看一下有沒有要改 |  |
| zh2-134 | `w8␣2k7vup␣5j0␣ru6tj␣xk7␣su3wu/␣xk7ao6␣tl␣cl3wu/␣` | 她的新專輯出了␣你聽了沒␣超好聽 | 他的新專輯出了␣你聽了沒␣超好聽 |  |
| zh2-140 | `jp6rm42u042k71u3cl32ji␣␣ao35␣2.␣vu;3a93` | 文具店的筆好多␣每支都想買 | 文具店的比好多␣每隻都想買 |  |
| zh2-161 | `t␣j06ul4cl3vu;3gjo4␣dk3s/6ul4vu0␣w;3u␣vu84` | 吃完藥好想睡␣可能要先躺一下 | 吃完要好想睡␣可能要先躺一下 |  |
| zh2-163 | `u,4ru␣1l41ul3yji4cl3xk7a87␣ru/␣xu3y94jp4` | 業績報表做好了嗎␣經理在問 | 業績爆表做好了嗎␣經理再問 |  |
| zh2-169 | `ql3j06a83x8␣nj/␣␣rul3nj0␣2l4y.31j42j/4` | 跑完馬拉松␣腳痠到走不動 | 跑完馬拉松␣腳酸到走不動 |  |
| zh2-190 | `5k4xu3u.3bp6yji4a87␣ji3dk3u3yji45k41u0␣a87` | 這裡有人坐嗎␣我可以坐這邊嗎 | 這裡有人做嗎␣我可以做這邊嗎 |  |
| zh2-197 | `u␣g/␣rul4ji3gl3t␣vu062k7␣dk3g4cl3s06` | 醫生叫我少吃鹹的␣可是好難 | 一聲叫我少吃鹹的␣可是好難 |  |
| zh2-200 | `vu0␣wu06gp␣fu/31ul3␣y94fm4-4cl4ejo4w96q962jo4` | 先填申請表␣再去二號櫃台排隊 | 先填申請表␣再去二號櫃檯排隊 |  |

</details>

<details><summary>spec-fullwidth：錯 2 句</summary>

| id | 按鍵 | 要 | 得到 | 備註 |
|---|---|---|---|---|
| fw-01 | `{ShiftSpace}hello␣` | ｈｅｌｌｏ　 | ␣hello␣ | P4 |
| fw-02 | `{ShiftSpace}gmail.com` | ｇｍａｉｌ．ｃｏｍ | ␣gmail.com | P4 |

</details>
