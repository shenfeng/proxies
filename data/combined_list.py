__author__ = 'feng'

import re
import logging
import argparse

FORMAT = '%(asctime)-15s  %(message)s'
logging.basicConfig(format=FORMAT, level=logging.INFO)

parser = argparse.ArgumentParser(description="Generate user vector and deal vector")
parser.add_argument('--dir', type=str, default="/home/feng/workspace/gocode/proxies", help='Directory root')
parser.add_argument('--result', type=str, default="result.txt", help='Result txt')
args = parser.parse_args()

results = open('%s/%s' % (args.dir, args.result), 'w')

space = re.compile(r'\s*')


def append_results(r):
    # print r
    results.write('%s\n' % r)


def dump_first_ip_then_port_http(lines):
    for line in lines:
        parts = space.split(line)
        s = 'http %s:%s' % (parts[0], parts[1])
        append_results(s)


def dump_proxy_net():
    lines = open('%s/google_proxy.net' % args.dir).readlines()
    results.write('# google_proxy.net\n')
    dump_first_ip_then_port_http(lines)


def dump_free_proxy_list_net():
    lines = open('%s/free_proxy_list.net' % args.dir).readlines()
    results.write('# free_proxy_list.net\n')
    dump_first_ip_then_port_http(lines)


def dump_hidemyass():
    lines = open('%s/hidemyass' % args.dir).readlines()
    results.write('# hidemyass')

    MINUTES = "minutes"
    SECS = 'secs'

    chunks = [lines[i:i + 2] for i in xrange(0, len(lines), 2)]
    for line1, line2 in chunks:
        proxy_type = space.split(line2)[0].lower()
        if MINUTES in line1:
            part = line1[line1.find(MINUTES) + len(MINUTES):].strip()
        elif SECS in line1:
            part = line1[line1.find(SECS) + len(SECS):].strip()

        parts = space.split(part)
        s = '%s %s:%s' % (proxy_type, parts[0], parts[1])
        append_results(s)


def dump_cn_proxy_com():
    lines = open('%s/cn_proxy_com' % args.dir).readlines()
    results.write('# cn_proxy_com')
    chunks = [lines[i:i + 2] for i in xrange(0, len(lines), 2)]
    for line1, line2 in chunks:
        ip, port, other = space.split(line1, 2)
        append_results('http %s:%s' % (ip, port))


def dump_freeproxylists_net():
    lines = open('%s/freeproxylists.net' % args.dir).readlines()
    results.write('# freeproxylists.net')
    for line in lines:
        ip, port, type, other = space.split(line, 3)
        s = '%s %s:%s' % (type.lower(), ip, port)
        append_results(s)


def dump_samair_ru():
    lines = open('%s/samair.ru_http' % args.dir).readlines()
    results.write('# samair.ru_http')
    for line in lines:
        proxy, other = space.split(line, 1)
        s = 'http %s' % proxy
        append_results(s)


def dump_cnproxy_com():
    lines = open('%s/cnproxy.com' % args.dir).readlines()
    results.write('# cnproxy.com')
    for line in lines:
        proxy, type, other = space.split(line, 2)
        s = '%s %s' % (type.lower(), proxy)
        append_results(s)


def cal_type_cout():
    results.flush()

    lines = open('%s/%s' % (args.dir, args.result)).readlines()
    unique = set()
    from collections import defaultdict, Counter

    c = Counter()
    for line in lines:
        if not line.startswith('#'):
            type, proxy = line.strip().split(' ')
            if proxy not in unique:
                unique.add(proxy)
                c[type] += 1

    print len(unique), c


def dump_proxy_ipcn_org():
    lines = open('%s/proxy_ipcn_org' % args.dir).readlines()
    results.write('# proxy_ipcn_org')
    for line in lines:
        append_results('http %s' % line.strip())


def dump_freeproxylists_net2():
    lines = open('%s/freeproxylists_net' % args.dir).readlines()
    results.write('# freeproxylists_net')
    for line in lines:
        ip, port, type, other = space.split(line, 3)
        append_results('%s %s:%s' % (type, ip, port))


if __name__ == '__main__':
    dump_proxy_net()
    dump_free_proxy_list_net()
    dump_hidemyass()
    dump_freeproxylists_net()
    dump_samair_ru()
    dump_cnproxy_com()
    dump_cn_proxy_com()
    dump_proxy_ipcn_org()
    dump_freeproxylists_net2()
    cal_type_cout()


