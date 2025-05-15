"""Works with the database."""

from typing import Any

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from numpy import typing as npt
from qcodes import dataset

from ..datatypes import Channel, Gate, Ohmic
from .statedf import StateDF


class ChannelData1D:
    """Contains the data for a particular channel.

    Args:
        xname (str): The name of the x-axis
        channel (Channel): The channel
        xarray (np.ndarray): The x-axis data
        yarray (np.ndarray): The y-axis data
    """

    xname: Gate | Ohmic | str
    channel: Channel
    xarray: npt.NDArray[np.floating[Any]] | None
    yarray: npt.NDArray[np.floating[Any]] | None

    def __init__(
        self,
        xname: Gate | Ohmic | str,
        channel: Channel,
        xarray: npt.NDArray[np.floating[Any]] | None = None,
        yarray: npt.NDArray[np.floating[Any]] | None = None,
    ):
        """Initializes a ChannelData object with the given parameters.

        Args:
            xname (Gate | Ohmic | str): The name of the x-axis.
            channel (Channel): The channel.
            xarray (npt.NDArray[np.floating[Any]], optional): The x-axis data. Defaults to None.
            yarray (npt.NDArray[np.floating[Any]]): The y-axis data.
        """
        self.xname = xname
        self.channel = channel
        self.xarray = xarray
        self.yarray = yarray

    def get_xarray(self) -> npt.NDArray[np.floating[Any]]:
        """Returns the x-axis data."""
        if self.xarray is None:
            raise ValueError("xarray is None")
        return self.xarray

    def get_yarray(self) -> npt.NDArray[np.floating[Any]]:
        """Returns the y-axis data."""
        if self.yarray is None:
            raise ValueError("yarray is None")
        return self.yarray


class ChannelData2D:
    """Contains the data for a particular channel.

    Args:
        xname (str): The name of the x-axis
        yname (str): The name of the y-axis
        channel (Channel): The channel
        xarray (np.ndarray): The x-axis data
        yarray (np.ndarray): The y-axis data
        zarray (np.ndarray): The z-axis data
    """

    xname: Gate | Ohmic | str
    yname: Gate | Ohmic | str
    channel: Channel
    xarray: npt.NDArray[np.floating[Any]] | None
    yarray: npt.NDArray[np.floating[Any]] | None
    zarray: npt.NDArray[np.floating[Any]] | None

    def __init__(
        self,
        xname: Gate | Ohmic | str,
        yname: Gate | Ohmic | str,
        channel: Channel,
        xarray: npt.NDArray[np.floating[Any]] | None = None,
        yarray: npt.NDArray[np.floating[Any]] | None = None,
        zarray: npt.NDArray[np.floating[Any]] | None = None,
    ):
        """Initializes a ChannelData object with the given parameters.

        Args:
            xname (Gate | Ohmic | str): The name of the x-axis.
            yname (Gate | Ohmic | str): The name of the y-axis.
            channel (Channel): The channel.
            xarray (npt.NDArray[np.floating[Any]], optional): The x-axis data. Defaults to None.
            yarray (npt.NDArray[np.floating[Any]]): The y-axis data.
            zarray (npt.NDArray[np.floating[Any]]): The z-axis data.
        """
        self.xname = xname
        self.yname = yname
        self.channel = channel
        self.xarray = xarray
        self.yarray = yarray
        self.zarray = zarray

    def get_xarray(self) -> npt.NDArray[np.floating[Any]]:
        """Returns the x-axis data."""
        if self.xarray is None:
            raise ValueError("xarray is None")
        return self.xarray

    def get_yarray(self) -> npt.NDArray[np.floating[Any]]:
        """Returns the y-axis data."""
        if self.yarray is None:
            raise ValueError("yarray is None")
        return self.yarray

    def get_zarray(self) -> npt.NDArray[np.floating[Any]]:
        """Returns the z-axis data."""
        if self.zarray is None:
            raise ValueError("zarray is None")
        return self.zarray


class Data1D:
    """Contains the data for a particular multi-channel measurement.

    Args:
        data (list[ChannelData1D]): The data for each channel
        channels (list[Channel]): The channels
    """

    value: list[ChannelData1D]
    channels: list[Channel]

    def __init__(self, data: list[ChannelData1D]):
        """Initializes a Data1D object with the given parameters."""
        self.value = data

    def store_channels(self):
        """Gets all of the channels and stores them in a list attribute."""
        self.channels = [channel.channel for channel in self.value]

    def append(self, data: ChannelData1D):
        """Appends a new ChannelData1D object to the Data1D object."""
        self.value.append(data)
        self.store_channels()

    def getChannelData(self, channel: Channel):
        """Returns the ChannelData1D object associated with the given channel."""
        for data in self.value:
            if data.channel == channel:
                return data


class DatabaseManager:
    """Manages the data from the database."""

    def __init__(self):
        """Initializes the DatabaseManager object."""
        pass

    def getSweep1DTrace(
        self,
        id: int,
        pads: list[Gate | Ohmic | Channel] | Channel,
        average: bool = True,
    ) -> ChannelData1D:
        """Returns a Sweep1D object based on pad input.

        Args:
            id: data run id to pass to qc.dataset.load_by_id()
            pads: the pad associated with the desired trace. Can only include 1 current channel.
            average: boolean specifying if the desired trace is averaged or not. False = no, True = yes.

        Returns:
            Data1D object containing the data for the sweep
        """
        if isinstance(pads, Channel):
            if average:
                raise ValueError("Cannot average a single channel")
            out = ChannelData1D(xname="time", channel=pads)
            pads = [pads]
        elif len(pads) == 1 and isinstance(pads[0], Channel):
            out = ChannelData1D(xname="time", channel=pads[0])
        elif len(pads) == 2:
            connection = next(
                (element for element in pads if isinstance(element, Gate | Ohmic)), None
            )
            if connection is None:
                raise ValueError("No current channel found")
            channel = next(
                (element for element in pads if isinstance(element, Channel)), None
            )
            if channel is None:
                raise ValueError("No voltage channel found")
            out = ChannelData1D(xname=connection, channel=channel)
        else:
            raise TypeError("Invalid typing for pads")
        data = dataset.load_by_id(id).get_parameter_data()
        for key in data.keys():
            if "aver" in key:
                if len(data[key].keys()) > 2:
                    raise ValueError("Sweep 1D is the only one implemented")

        index_df = self.reconstruct_df("index_df", id)
        state_df = StateDF(df=self.reconstruct_df("state_df", id))
        if len([item for item in pads if isinstance(item, Channel)]) > 1:
            raise ValueError("Gave two current values... Make up your mind!")
        rawpads = [pad.name if isinstance(pad, Channel) else pad.value for pad in pads]
        for pad in pads:
            # query1 = ""
            # query2 = ""
            # Current amplifier channel
            if isinstance(pad, Channel):
                nidaq = str(list(index_df[index_df["Pad"] == pad.name]["NIDaq"])[0])
                ni_ch = str(list(index_df[index_df["Pad"] == pad.name]["NIDaq Ch"])[0])
                if average:
                    fav1 = state_df.get_favorite1()
                    deps1 = state_df.get_deps1()
                    if any(item in [fav1] for item in rawpads) and any(
                        item2 in deps1 for item2 in rawpads
                    ):
                        raise ValueError(
                            "Gave both a favorite and dependant on the same axis"
                        )

                    query1 = nidaq + "_" + ni_ch + "_aver_voltage"
                    query2 = nidaq + "_bin_axis"
                    # Return the current channel
                    out.yarray = data[query1][query1][0]  # only for sweep1D
                    # Now investigate the rest of the pads
                    if fav1 in rawpads:
                        out.xarray = 1000 * data[query1][query2][0]
                        return out
                    for dep in state_df.get_deps1():
                        if dep in rawpads:
                            (start, stop) = state_df.get_sweep_start_end(pad=dep)
                            npoints = len(data[query1][query2][0])
                            out.xarray = np.linspace(start, stop, num=npoints)
                            return out
                else:
                    query1 = nidaq + "_" + ni_ch + "_" + "voltage_traces"
                    query2 = nidaq + "_time_axis"
                    out.xarray = data[query1][query2][0]
                    out.yarray = data[query1][query1][0]
                    return out
        raise ValueError("Invalid pad inputs. This should not run!")

    def get_Sweep2D(
        self,
        id: int,
        pads: list[Gate | Ohmic | Channel],
        average: bool = True,
        invert: bool = False,
    ) -> ChannelData2D:
        """Returns a Sweep2D object based on pad input. No turbo support.

        Args:
            id: data run id to pass to qc.dataset.load_by_id()
            pads: the pad associated with the desired trace. Can only include 1 current channel.
            average: boolean specifying if the desired trace is averaged or not. False = no, True = yes.
            invert: boolean specifying if the desired trace is inverted or not. False = no, True = yes.

        Returns:
            Data2D object containing the data for the sweep
        """
        if not average:
            if len(pads) != 2:
                raise ValueError("A channel and pad needs to be specified")
            channel = next(
                (element for element in pads if isinstance(element, Channel)), None
            )
            if channel is None:
                raise ValueError("No current channel found")
            pad = next(
                (element for element in pads if isinstance(element, Gate | Ohmic)), None
            )
            if pad is None:
                raise ValueError("No voltage channel found")
            out = ChannelData2D(xname="time", channel=channel, yname=pad)
        elif len(pads) == 3:
            # normal
            connection = next(
                (element for element in pads if isinstance(element, Gate | Ohmic)), None
            )
            if connection is None:
                raise ValueError("No Gate or Ohmic found")
            nextconnection = next(
                (
                    element
                    for element in pads
                    if isinstance(element, Gate | Ohmic) and element != connection
                ),
                None,
            )
            if nextconnection is None:
                raise ValueError("No Gate or Ohmic found")
            channel = next(
                (element for element in pads if isinstance(element, Channel)), None
            )
            if channel is None:
                raise ValueError("No current channel found")
            out = ChannelData2D(xname=connection, yname=nextconnection, channel=channel)
        else:
            raise TypeError("Invalid typing for pads")
        data = dataset.load_by_id(id).get_parameter_data()
        for key in data.keys():
            if "aver" in key:
                if len(data[key].keys()) > 3 or len(data[key].keys()) < 2:
                    raise ValueError("Sweep 2D is the only one implemented")

        index_df = self.reconstruct_df("index_df", id)
        state_df = StateDF(df=self.reconstruct_df("state_df", id))

        fav2 = state_df.get_favorite2()

        deps2 = state_df.get_deps2()

        if len([item for item in pads if isinstance(item, Channel)]) > 1:
            raise ValueError("Gave two current values... Make up your mind!")
        rawpads = [pad.name if isinstance(pad, Channel) else pad.value for pad in pads]
        for pad in pads:
            # query1 = ""
            # query2 = ""
            # Current amplifier channel
            if isinstance(pad, Channel):
                nidaq = str(list(index_df[index_df["Pad"] == pad.name]["NIDaq"])[0])
                ni_ch = str(list(index_df[index_df["Pad"] == pad.name]["NIDaq Ch"])[0])
                if average:
                    fav1 = state_df.get_favorite1()
                    deps1 = state_df.get_deps1()
                    if any(item in [fav1] for item in rawpads) and any(
                        item2 in deps1 for item2 in rawpads
                    ):
                        raise ValueError(
                            "Gave both a favorite and dependant on the same axis"
                        )
                    query1 = nidaq + "_" + ni_ch + "_aver_voltage"
                    query2 = nidaq + "_bin_axis"
                    # Return the current channel
                    out.zarray = data[query1][query1].T
                    if invert:
                        out.zarray = out.zarray.T
                    qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                    Q_ch = str(list(index_df[index_df["Pad"] == fav2]["Channel"])[0])
                    query3 = qdac + "_chan" + Q_ch + "_v"
                    if fav2 in rawpads:  # sweep2D case
                        out.yarray = 1000.0 * data[query1][query3].T
                    if fav1 in rawpads:
                        out.xarray = 1000.0 * data[query1][query2].T
                    # Now investigate the rest of the pads
                    for dep in state_df.get_deps1():
                        if dep in rawpads:
                            (start, stop) = state_df.get_sweep_start_end(pad=dep)
                            npoints = len(data[query1][query2][0])
                            out.xarray = np.tile(
                                np.linspace(start, stop, num=npoints),
                                (len(data[query1][query3][0]), 1),
                            ).T
                    for dep in state_df.get_deps2():
                        if dep in rawpads:
                            (start, stop) = state_df.get_sweep_start_end(pad=dep)
                            npoints = len(data[query1][query3][0])
                            out.yarray = np.tile(
                                np.linspace(start, stop, num=npoints),
                                (len(data[query1][query2][0]), 1),
                            )
                    return out
                else:
                    query1 = nidaq + "_" + ni_ch + "_" + "voltage_traces"
                    query2 = nidaq + "_time_axis"
                    qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                    Q_ch = str(list(index_df[index_df["Pad"] == fav2]["Channel"])[0])
                    query3 = qdac + "_chan" + Q_ch + "_v"
                    out.xarray = data[query1][query2][0]
                    out.zarray = data[query1][query1][0]
                    assert out.zarray is not None
                    if invert:
                        out.zarray = out.zarray.T
                    if fav2 in rawpads:
                        out.yarray = 1000.0 * data[query1][query3].T
                    for dep in deps2:
                        if dep in rawpads:
                            (start, stop) = state_df.get_sweep_start_end(pad=dep)
                            npoints = len(data[query1][query3][0])
                            # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                            out.yarray = np.tile(
                                np.linspace(start, stop, num=npoints),
                                (len(data[query1][query2][0]), 1),
                            )
                    return out
        raise ValueError("Invalid pad inputs. This should not run!")

    def getTrace(
        self,
        id: int,
        pads: list[Gate | Ohmic | Channel] | Gate | Ohmic | Channel,
        average: bool = True,
    ) -> dict:
        """Returns a trace based on pad input.

        TODO: test on sweep1D and plot1D
        TODO: update unaveraged sweep2D and plot2D
        TODO: label plots correctly from station labels when pulling from database

        Args:
            id: data run id to pass to qc.dataset.load_by_id()
            pads: the pads associated with the desired trace. Can only include 1 current channel.
            average: boolean specifying if the desired trace is averaged or not. False = no, True = yes.
        """
        returnDict = {}
        # if isinstance(pads, str):
        #     pads = pads.split(";")

        if isinstance(pads, (Gate | Ohmic | Channel)):
            pads = [pads]

        TwoDsweep = False
        sweep2d = False
        turbo = False
        sweep1d = False

        data = dataset.load_by_id(id).get_parameter_data()
        # Determine if 1D or 2D sweep
        for key in data.keys():
            if "aver" in key:
                if len(data[key].keys()) > 2:
                    TwoDsweep = True
                    if "turbo" in key:
                        turbo = True
                    elif "aver_voltage" in key:
                        sweep2d = True
                elif len(data[key].keys()) <= 2:
                    sweep1d = True
            # else: assume you are a plot 2d
        index_df = self.reconstruct_df("index_df", id)
        state_df = self.reconstruct_df("state_df", id)
        if average:
            # Figuring out favorites
            fav1 = list(state_df[state_df["Favorite1"] == "fav"]["Pads"])[0]
            deps1 = list(state_df[state_df["Favorite1"] == "dep"]["Pads"])
            fav2 = []
            deps2 = []
            if TwoDsweep:  # Filling the proper values in only if 2D sweep
                fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                deps2 = list(state_df[state_df["Favorite2"] == "dep"]["Pads"])

            # Error detection
            if TwoDsweep:
                if any(item in fav2 for item in pads) and any(
                    item2 in deps2 for item2 in pads
                ):
                    raise ValueError(
                        "Gave both a favorite and dependant on the same axis"
                    )
            if any(item in fav1 for item in pads) and any(
                item2 in deps1 for item2 in pads
            ):
                raise ValueError("Gave both a favorite and dependant on the same axis")
            if len([item for item in pads if isinstance(item, Channel)]) > 1:
                raise ValueError("Gave two current values... Make up your mind!")

        # First we need to check for the current amplifier to pull, and get y data for 1d or z data for 2d
        for pad in pads:
            # query1 = ""
            # query2 = ""
            # Current amplifier channel
            pad = str(pad)
            if pad[0] == "I":
                nidaq = str(list(index_df[index_df["Pad"] == pad]["NIDaq"])[0])
                ni_ch = str(list(index_df[index_df["Pad"] == pad]["NIDaq Ch"])[0])
                if average and (sweep2d or sweep1d):  # sweep2d or sweep1d averaged
                    fav1 = list(state_df[state_df["Favorite1"] == "fav"]["Pads"])[0]
                    query1 = nidaq + "_" + ni_ch + "_aver_voltage"
                    query2 = nidaq + "_bin_axis"
                    # Return the current channel
                    returnDict[pad] = data[query1][query1][0]  # only for sweep1D
                    # Now investigate the rest of the pads
                    if sweep2d:
                        returnDict[pad] = data[query1][query1].T
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                        Q_ch = str(
                            list(index_df[index_df["Pad"] == fav2]["Channel"])[0]
                        )
                        query3 = qdac + "_chan" + Q_ch + "_v"
                    if fav1 in pads:
                        if TwoDsweep:
                            returnDict[fav1] = 1000 * data[query1][query2].T
                        else:
                            returnDict[fav1] = 1000 * data[query1][query2][0]
                    if sweep2d:
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                        Q_ch = str(
                            list(index_df[index_df["Pad"] == fav2]["Channel"])[0]
                        )
                        query3 = qdac + "_chan" + Q_ch + "_v"
                        if fav2 in pads:  # sweep2D case
                            returnDict[fav2] = 1000 * data[query1][query3].T
                    deps1 = list(state_df[state_df["Favorite1"] == "dep"]["Pads"])
                    for dep in deps1:
                        if dep in pads:
                            sweep = list(
                                state_df[state_df["Pads"] == dep][
                                    "Sweep Start->End (mV)"
                                ]
                            )[0]
                            start = float(sweep[: sweep.find("->")])
                            stop = float(sweep[sweep.find(">") + 1 :])
                            npoints = len(data[query1][query2][0])
                            if sweep1d:
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.linspace(start, stop, num=npoints)
                            else:  # sweep2D case
                                fav2 = list(
                                    state_df[state_df["Favorite2"] == "fav"]["Pads"]
                                )[0]
                                qdac = str(
                                    list(index_df[index_df["Pad"] == fav2]["QDac"])[0]
                                )
                                Q_ch = str(
                                    list(index_df[index_df["Pad"] == fav2]["Channel"])[
                                        0
                                    ]
                                )
                                query3 = qdac + "_chan" + Q_ch + "_v"
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    (len(data[query1][query3][0]), 1),
                                ).T
                    if sweep2d:
                        deps2 = list(state_df[state_df["Favorite2"] == "dep"]["Pads"])
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                        Q_ch = str(
                            list(index_df[index_df["Pad"] == fav2]["Channel"])[0]
                        )
                        query3 = qdac + "_chan" + Q_ch + "_v"
                        for dep in deps2:  # sweep2D case
                            if dep in pads:
                                sweep = list(
                                    state_df[state_df["Pads"] == dep][
                                        "Sweep Start->End (mV)"
                                    ]
                                )[0]
                                start = float(sweep[: sweep.find("->")])
                                stop = float(sweep[sweep.find(">") + 1 :])
                                npoints = len(data[query1][query3][0])
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    (len(data[query1][query2][0]), 1),
                                )
                elif not average and (
                    sweep1d or sweep2d
                ):  # sweep2d or sweep1d time axis
                    query1 = nidaq + "_" + ni_ch + "_" + "voltage_traces"
                    query2 = nidaq + "_time_axis"
                    if sweep1d:
                        returnDict["time"] = data[query1][query2][0]
                        returnDict[pad + "(t)"] = data[query1][query1][0]
                    else:  # sweep2d
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        deps2 = list(state_df[state_df["Favorite2"] == "dep"]["Pads"])
                        qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                        Q_ch = str(
                            list(index_df[index_df["Pad"] == fav2]["Channel"])[0]
                        )
                        query3 = qdac + "_chan" + Q_ch + "_v"
                        returnDict["time"] = data[query1][query2]
                        returnDict[pad + "(t)"] = data[query1][query1]
                        # sweep2D special third index
                        if fav2 in pads:
                            returnDict[fav2] = 1000 * data[query1][query3]
                        for dep in deps2:
                            if dep in pads:
                                sweep = list(
                                    state_df[state_df["Pads"] == dep][
                                        "Sweep Start->End (mV)"
                                    ]
                                )[0]
                                start = float(sweep[: sweep.find("->")])
                                stop = float(sweep[sweep.find(">") + 1 :])
                                npoints = len(data[query1][query3][0])
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    (len(data[query1][query2][0]), 1),
                                )
                elif average and turbo:  # turbo sweep 2d averaged data
                    fav1 = list(state_df[state_df["Favorite1"] == "fav"]["Pads"])[0]
                    deps1 = list(state_df[state_df["Favorite1"] == "dep"]["Pads"])
                    fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                    deps2 = list(state_df[state_df["Favorite2"] == "dep"]["Pads"])
                    query1 = nidaq + "_" + ni_ch + "_aver_turbo_array"
                    query2 = nidaq + "_turbo_x_bin_array"
                    query3 = nidaq + "_turbo_y_bin_array"
                    # Return the current channel
                    returnDict[pad] = data[query1][query1][0]

                    # Now investigate the rest of the pads
                    if fav1 in pads:
                        returnDict[fav1] = 1000 * data[query1][query2][0]
                    elif fav1 not in pads:
                        for dep in deps1:
                            if dep in pads:
                                sweep = list(
                                    state_df[state_df["Pads"] == dep][
                                        "Sweep Start->End (mV)"
                                    ]
                                )[0]
                                start = float(sweep[: sweep.find("->")])
                                stop = float(sweep[sweep.find(">") + 1 :])
                                npoints = len(data[query1][query2][0])
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    (len(data[query1][query3][0]), 1),
                                ).T
                    if fav2 in pads:
                        returnDict[fav2] = 1000 * data[query1][query3][0]
                    elif fav2 not in pads:
                        for dep in deps2:
                            if dep in pads:
                                sweep = list(
                                    state_df[state_df["Pads"] == dep][
                                        "Sweep Start->End (mV)"
                                    ]
                                )[0]
                                start = float(sweep[: sweep.find("->")])
                                stop = float(sweep[sweep.find(">") + 1 :])
                                npoints = len(data[query1][query3][0])
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    (len(data[query1][query2][0]), 1),
                                )
                elif not average and turbo:  # turbo time axis
                    query1 = nidaq + "_" + ni_ch + "_turbo_array"
                    query2 = nidaq + "_turbo_time"
                    returnDict["time"] = data[query1][query2]
                    returnDict[pad + "(t)"] = data[query1][query1]
                elif average:  # assume plot2d or plot1d
                    query1 = nidaq + "_" + ni_ch + "_aver_value"
                    fav1 = list(state_df[state_df["Favorite1"] == "fav"]["Pads"])[0]
                    deps1 = list(state_df[state_df["Favorite1"] == "dep"]["Pads"])
                    qdac = str(list(index_df[index_df["Pad"] == fav1]["QDac"])[0])
                    Q_ch = str(list(index_df[index_df["Pad"] == fav1]["Channel"])[0])
                    query2 = qdac + "_chan" + Q_ch + "_v"
                    returnDict[pad] = data[query1][query1]
                    # Now investigate the rest of the pads
                    if TwoDsweep:
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        qdac = str(list(index_df[index_df["Pad"] == fav2]["QDac"])[0])
                        Q_ch = str(
                            list(index_df[index_df["Pad"] == fav2]["Channel"])[0]
                        )
                        query3 = qdac + "_chan" + Q_ch + "_v"
                    if fav1 in pads:
                        if TwoDsweep:
                            npoints = len(np.unique(data[query1][query2]))
                            returnDict[fav1] = 1000 * data[query1][query2].reshape(
                                npoints, -1
                            )
                            returnDict[pad] = returnDict[pad].reshape(npoints, -1)
                        else:  # 1D sweep
                            returnDict[fav1] = 1000 * data[query1][query2]
                    if TwoDsweep:
                        query3 = qdac + "_chan" + Q_ch + "_v"
                        fav2 = list(state_df[state_df["Favorite2"] == "fav"]["Pads"])[0]
                        if fav2 in pads:  # sweep2D case
                            npoints = len(np.unique(data[query1][query2]))
                            returnDict[fav2] = 1000 * data[query1][query3].reshape(
                                -1, npoints
                            )
                    for dep in deps1:
                        if dep in pads:
                            sweep = list(
                                state_df[state_df["Pads"] == dep][
                                    "Sweep Start->End (mV)"
                                ]
                            )[0]
                            start = float(sweep[: sweep.find("->")])
                            stop = float(sweep[sweep.find(">") + 1 :])
                            npoints = len(np.unique(data[query1][query2]))
                            if sweep1d:
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.linspace(start, stop, num=npoints)
                            else:  # sweep2D case
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.repeat(
                                    np.linspace(start, stop, num=npoints),
                                    int(len(data[query1][query1]) / npoints),
                                ).reshape(npoints, -1)
                                returnDict[pad] = returnDict[pad].reshape(npoints, -1)
                    if TwoDsweep:
                        query3 = qdac + "_chan" + Q_ch + "_v"
                        deps2 = list(state_df[state_df["Favorite2"] == "dep"]["Pads"])
                        for dep in deps2:  # sweep2D case
                            if dep in pads:
                                sweep = list(
                                    state_df[state_df["Pads"] == dep][
                                        "Sweep Start->End (mV)"
                                    ]
                                )[0]
                                start = float(sweep[: sweep.find("->")])
                                stop = float(sweep[sweep.find(">") + 1 :])
                                npoints = len(np.unique(data[query1][query3]))
                                # print("start = " + str(start) + "\n Stop = " + str(stop) + "\n npoints = " + str(npoints))
                                returnDict[dep] = np.tile(
                                    np.linspace(start, stop, num=npoints),
                                    int(len(data[query1][query1]) / npoints),
                                ).reshape(-1, npoints)
                elif not average:  # assume non averaged plot1d or plot2d
                    query1 = nidaq + "_" + ni_ch + "_voltage_traces"
                    returnDict[pad + "(t)"] = data[query1][query1]
                    query2 = nidaq + "_time_axis"
                    returnDict["time"] = data[query1][query2]
        return returnDict

    def reconstruct_df(self, name, dataset_id):
        """Reconstructs the metadata from the excel file.

        It gets stored in string format alongside the data
        Returns index_df which is wiremap of sample + fridge

        Args:
            dataset_id : the ID in the currently selected database corresponding to the querried data
            name : the name of the dataframe to reconstruct
        """
        data = dataset.load_by_id(dataset_id)
        string = data.metadata[name]
        columnDivider = string.split("//")
        # print("c: " + str(columnDivider))
        df = pd.DataFrame(columns=columnDivider[0].split("@@"))
        for row in range(len(columnDivider) - 2):
            # print(columnDivider[row+1].split('@@'))
            df.loc[row] = columnDivider[row + 1].split("@@")
        return df

    def plot_leakage_matrix(self, dataset_id: int):
        """Plots a leakage matrix that was stored in the database.

        The axis labels were stored inside the instrument and can be pulled out to actually diagnose the plot properly

        Args:
            dataset_id : the ID in the currently selected database corresponding to the querried data
        """
        snapshot = dataset.load_by_id(dataset_id).snapshot
        if snapshot is None:
            raise ValueError("snapshot is detected to be None")
        QDACVchannels = snapshot["station"]["instruments"]["leaky"]["parameters"][
            "channelNames"
        ]["value"]
        Resistances = dataset.load_by_id(dataset_id).get_parameter_data()[
            "leaky_Resistance_matrix"
        ]["leaky_Resistance_matrix"][0]

        fig, ax = plt.subplots(figsize=(15, 15))
        plt.title("Gate Leakage")
        img = ax.imshow(Resistances, interpolation="none", vmin=0)
        ticks = np.arange(len(QDACVchannels))
        minorticks = np.arange(-0.5, len(ticks), 1)
        ax.set_xticks(ticks, labels=QDACVchannels, rotation=90)
        ax.set_yticks(ticks, labels=QDACVchannels)
        ax.set_xticks(minorticks, minor=True)
        ax.set_yticks(minorticks, minor=True)
        ax.grid(which="minor", color="grey", linewidth=1.5)
        plt.plot(
            [-0.5, len(QDACVchannels) - 0.5],
            [-0.5, len(QDACVchannels) - 0.5],
            color="red",
            linewidth=0.5,
        )
        colorbar = fig.colorbar(img)
        colorbar.set_label("Resistance (MΩ)")
        plt.show()
        return fig
