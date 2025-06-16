#ifndef UHD_NMEA_H
#define UHD_NMEA_H

#include <uhd/usrp/multi_usrp.hpp>
#include <sstream>
#include <vector>
#include <cmath>

struct GpsFix {
  double lat  = NAN;   // +‑90  (deg, WGS‑84, decimal)
  double lon  = NAN;   // +‑180 (deg, WGS‑84, decimal)
  double alt  = NAN;   // metres above mean sea‑level (GPGGA field 9)
  bool   ok   = false; // quality flag
};

static double dm_to_deg(const std::string& dm)
{
  // "ddmm.mmmm"  or  "dddmm.mmmm"
  if (dm.empty()) return NAN;
  double v = std::stod(dm);
  int    deg = int(v / 100);
  double min = v - deg * 100;
  return deg + min / 60.0;
}

static GpsFix parse_gpgga(const std::string& nmea)
{
  GpsFix fix;
  if (nmea.size() < 6 || nmea.substr(0, 6) != "$GPGGA") return fix;

  std::vector<std::string> f;
  std::stringstream ss(nmea);
  std::string token;
  while (std::getline(ss, token, ',')) f.push_back(token);
  if (f.size() < 10) return fix;

  /* field numbers (0‑based):
      2 = latitude (ddmm.mmmm)
      3 = N / S
      4 = longitude (dddmm.mmmm)
      5 = E / W
      6 = fix quality   ("0" = invalid)
      9 = altitude (m)
  */
  if (f[6] == "0") return fix;          // no valid fix

  fix.lat = dm_to_deg(f[2]);
  if (f[3] == "S") fix.lat = -fix.lat;

  fix.lon = dm_to_deg(f[4]);
  if (f[5] == "W") fix.lon = -fix.lon;

  fix.alt = std::stod(f[9]);            // metres
  fix.ok  = true;
  return fix;
}

#endif